//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.
package wire

import (
	"encoding/json"
	"fmt"
	"github.com/google/wire"
	"log"
	"xixunyunsign/autosign"
	"xixunyunsign/platformauth"
	"xixunyunsign/service"
	"xixunyunsign/service/impl"
	"xixunyunsign/utils"
	"xixunyunsign/web"
)

// --- Command Runners ---

// LoginCmdRunner holds dependencies for the login command.
type LoginCmdRunner struct {
	AuthService service.AuthService
}

// NewLoginCmdRunner creates a new runner for the login command.
func NewLoginCmdRunner(authService service.AuthService) *LoginCmdRunner {
	return &LoginCmdRunner{AuthService: authService}
}

// QueryCmdRunner holds dependencies for query commands.
type QueryCmdRunner struct {
	QueryService service.QueryService
}

// NewQueryCmdRunner creates a new runner for query commands.
func NewQueryCmdRunner(queryService service.QueryService) *QueryCmdRunner {
	return &QueryCmdRunner{QueryService: queryService}
}

// RunSearchSchool executes the search school logic.
// This method belongs to the QueryCmdRunner.
func (r *QueryCmdRunner) RunSearchSchool(schoolName string) {
	schools, err := r.QueryService.SearchSchool(schoolName)
	if err != nil {
		log.Printf("查询学校失败: %v\n", err)
		return
	}
	if len(schools) == 0 {
		fmt.Println("未找到匹配的学校。")
		return
	}
	jsonData, _ := json.MarshalIndent(schools, "", "  ")
	fmt.Println(string(jsonData))
}

// SignCmdRunner holds dependencies for the sign command.
type SignCmdRunner struct {
	SignService service.SignService
}

// NewSignCmdRunner creates a new runner for the sign command.
func NewSignCmdRunner(signService service.SignService) *SignCmdRunner {
	return &SignCmdRunner{SignService: signService}
}

// --- Provider Sets ---

// Provider sets define how to build components

// RepositorySet provides database and user repository implementations.
var RepositorySet = wire.NewSet(
	autosign.NewStore,
	utils.ProvideDB,
	utils.NewUserRepository,
)

// ServiceSet provides service implementations, depending on repositories.
var ServiceSet = wire.NewSet(
	impl.NewAuthService,
	impl.NewQueryService,
	impl.NewSignService,
	impl.NewReportService,
)

// HandlerSet provides the web handlers, depending on services.
var HandlerSet = wire.NewSet(
	platformauth.ProvideStore,
	web.NewHandlers,
)

// RouterSet provides the Gin engine, depending on handlers.
var RouterSet = wire.NewSet(
	web.SetupRouter,
)

// ServerSet provides the web server, depending on the router.
var ServerSet = wire.NewSet(
	web.NewServer,
)

// --- Initializers ---

// InitializeServer creates a new web server with all dependencies injected.
func InitializeServer() (*web.Server, error) {
	wire.Build(
		RepositorySet,
		ServiceSet,
		HandlerSet,
		RouterSet,
		ServerSet,
	)
	return nil, nil
}

// InitializeLoginCmdRunner creates a new runner for the login command with dependencies.
func InitializeLoginCmdRunner() (*LoginCmdRunner, error) {
	wire.Build(
		RepositorySet,
		ServiceSet,
		NewLoginCmdRunner,
	)
	return nil, nil
}

// InitializeQueryCmdRunner creates a new runner for the query commands with dependencies.
func InitializeQueryCmdRunner() (*QueryCmdRunner, error) {
	wire.Build(
		RepositorySet,
		ServiceSet,
		NewQueryCmdRunner,
	)
	return nil, nil
}

// InitializeSignCmdRunner creates a new runner for the sign command with dependencies.
func InitializeSignCmdRunner() (*SignCmdRunner, error) {
	wire.Build(
		RepositorySet,
		ServiceSet,
		NewSignCmdRunner,
	)
	return nil, nil
}
