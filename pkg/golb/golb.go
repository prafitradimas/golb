package golb

import (
	"context"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

const (
	MaxRetryCount   int = 3
	MaxAttemptCount int = 3
)

const (
	RetryContextKey   string = "retry-ctx"
	AttemptContextKey string = "attempt-ctx"
)

type Server struct {
	Name         string
	URL          *url.URL
	Alive        bool
	ReverseProxy *httputil.ReverseProxy

	mu   sync.RWMutex
	pool *ServerPool
}

func (s *Server) IsAlive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Alive
}

func (s *Server) SetAlive(alive bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Alive = alive
}

func (s *Server) getRetryCount(req *http.Request) int {
	if retries, ok := req.Context().Value(RetryContextKey).(int); ok {
		return retries
	}
	return 0
}

func (s *Server) getAttemptCount(req *http.Request) int {
	if retries, ok := req.Context().Value(AttemptContextKey).(int); ok {
		return retries
	}
	return 0
}

func (s *Server) FallbackHandler(res http.ResponseWriter, req *http.Request) {
	attempts := s.getAttemptCount(req)
	if attempts >= MaxAttemptCount {
		log.Printf("%s(%s) Max attempts reached\n", s.Name, s.URL)
		http.Error(res, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}

	log.Printf("%s(%s) Attempt: %d\n", s.Name, s.URL, attempts)
	ctx := context.WithValue(req.Context(), AttemptContextKey, attempts+1)
	s.pool.ServeHTTP(res, req.WithContext(ctx))
}

func (s *Server) ErrorHandler(res http.ResponseWriter, req *http.Request, err error) {
	log.Printf("%s(%s) %s\n", s.Name, s.URL, err)

	retries := s.getRetryCount(req)
	if retries < MaxRetryCount {
		time.Sleep(time.Millisecond * 10)

		retries += 1
		log.Printf("%s(%s) Retry: %d\n", s.Name, s.URL, retries)
		ctx := context.WithValue(req.Context(), RetryContextKey, retries)
		s.ReverseProxy.ServeHTTP(res, req.WithContext(ctx))
		return
	}

	s.SetAlive(false)
	attempts := s.getAttemptCount(req)
	ctx := context.WithValue(req.Context(), AttemptContextKey, attempts+1)
	s.FallbackHandler(res, req.WithContext(ctx))
}

func NewServer(addr string) *ServerPool {
	sp := &ServerPool{
		Servers: []*Server{},
	}
	return sp
}

type ServerPool struct {
	Servers []*Server
	curr    uint64
}

func (sp *ServerPool) AddServer(name, rawUrl string) error {
	s := &Server{
		Name:  name,
		Alive: true,

		pool: sp,
	}

	url, err := url.Parse(rawUrl)
	if err != nil {
		return err
	}

	s.URL = url
	s.ReverseProxy = httputil.NewSingleHostReverseProxy(url)
	s.ReverseProxy.ErrorHandler = s.ErrorHandler

	_ = http.HandlerFunc(s.ReverseProxy.ServeHTTP)

	sp.Servers = append(sp.Servers, s)

	return nil
}

func (sp *ServerPool) NextIndex() int {
	return int(atomic.AddUint64(&sp.curr, 1) % uint64(len(sp.Servers)))
}

func (sp *ServerPool) Next() *Server {
	n := len(sp.Servers)
	next := sp.NextIndex()
	length := n + next

	for i := next; i < length; i++ {
		idx := i % n

		if sp.Servers[idx].IsAlive() {
			if i != next {
				atomic.StoreUint64(&sp.curr, uint64(idx))
			}
			return sp.Servers[idx]
		}
	}

	return nil
}

func (sp *ServerPool) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	peer := sp.Next()
	if peer != nil {
		peer.ReverseProxy.ServeHTTP(res, req)
		return
	}

	http.Error(res, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
}
