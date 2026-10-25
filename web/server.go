package web

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"xixunyunsign/autosign"
	// "github.com/gin-contrib/cors" // Moved CORS setup
	"github.com/gin-gonic/gin"
	"log"
	// "time" // No longer needed here
)

type Server struct {
	Router     *gin.Engine
	AutoRunner *autosign.Runner
}

// NewServer creates a new web server instance using the provided Gin engine.
func NewServer(engine *gin.Engine, handlers *Handlers) *Server {
	// CORS middleware should ideally be applied where the router/engine is created,
	// for example, within SetupRouter or by the injector that provides the engine.
	// If we keep it here, it assumes the passed engine hasn't had CORS applied yet.
	// Let's assume for now it's applied during router setup.

	// Example if CORS needed to be applied here:
	// engine.Use(cors.New(cors.Config{
	// 	AllowOrigins:     []string{"*"},
	// 	AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
	// 	AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Authorization"},
	// 	ExposeHeaders:    []string{"Content-Length"},
	// 	AllowCredentials: true,
	// 	MaxAge:           12 * time.Hour,
	// }))

	return &Server{
		AutoRunner: autosign.NewRunner(handlers.AutoSign, handlers.SignService, time.Now()),
		Router:     engine, // Use the provided engine
	}
}

func (s *Server) Run(addr string) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if s.AutoRunner != nil {
		go s.AutoRunner.Run(ctx)
	}
	log.Printf("启动Web服务器，监听地址: %s", addr)
	server := &http.Server{
		Addr: addr, Handler: s.Router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       45 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	go func() {
		<-ctx.Done()
		shutdown, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		_ = server.Shutdown(shutdown)
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Web服务器启动失败: %v", err)
	}
}
