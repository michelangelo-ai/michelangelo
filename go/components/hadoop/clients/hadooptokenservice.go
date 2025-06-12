package clients

import (
	"context"

	"github.com/pkg/errors"
	"go.uber.org/fx"
)

const (
	contentType             = "application/json"
	hadoopTokenServiceURL   = "http://localhost:5436/api/v1/tokens/"
	tokenServiceName        = "hadoop-token-service"
	timeoutSeconds          = 30
	successStatusCode       = 200
	internalErrorStatusCode = 500
)

// ErrPartialTokens represents an error when the token service returns partial tokens
var ErrPartialTokens = errors.New("hadoop token service returned partial tokens")

// Gateway for Hadoop Token Service
// API Doc: https://docs.google.com/document/d/1hg0YsiQjJfNv_tBlYOoE8d4xOdgID66NU1DBi0kgKH0/edit#heading=h.jdgmc56kigy2
type Gateway interface {
	GenerateTokens(ctx context.Context, request *GenerateTokensRequest) (resp *GenerateTokensResponse, err error)
	CancelTokens(ctx context.Context, request *CancelTokensRequest) (resp *CancelTokensResponse, err error)
	RenewTokens(ctx context.Context, request *RenewTokensRequest) (resp *RenewTokensResponse, statusCode int, err error)
	RenewTokensHTDev(ctx context.Context, request *RenewTokensRequest) (resp *RenewTokensResponse, err error)
}

type gateway struct {
}

// Params for creating Hadoop Token Service gateway
type Params struct {
	fx.In
}

// NewGateway creates a new HadoopTokenService gateway
func NewGateway(params Params) Gateway {
	return &gateway{}
}

// GenerateTokenParams for generating a token
type GenerateTokenParams struct {
	ServiceType   string `json:"serviceType"`
	ServiceRegion string `json:"serviceRegion"`
	ServiceAlias  string `json:"serviceAlias"`
	User          string `json:"user"`
	TokenKind     string `json:"tokenKind"`
	Renewer       string `json:"renewer"`
}

// GenerateTokensRequest for generate tokens requests
type GenerateTokensRequest struct {
	Params []GenerateTokenParams
}

// Token for delegation token
type Token struct {
	ServiceType   string `json:"serviceType"`
	ServiceRegion string `json:"serviceRegion"`
	ServiceAlias  string `json:"serviceAlias"`
	Token         string `json:"token"`
}

// GenerateTokensResponse for generate tokens response
type GenerateTokensResponse struct {
	Tokens      []Token `json:"tokens"`
	Credentials string  `json:"credentials"`
}

// RenewTokensRequest for renew tokens requests
type RenewTokensRequest struct {
	Tokens []Token
}

// RenewTokensResponse for renew tokens response
type RenewTokensResponse struct {
	NewExpirations []string
}

// CancelTokensRequest for cancel tokens
type CancelTokensRequest struct {
	Tokens []Token
}

// CancelTokensResponse for generate tokens response
type CancelTokensResponse struct {
}

func (g *gateway) GenerateTokens(ctx context.Context, request *GenerateTokensRequest) (resp *GenerateTokensResponse, err error) {
	return nil, nil
}

func (g *gateway) RenewTokens(ctx context.Context, request *RenewTokensRequest) (resp *RenewTokensResponse, statusCode int, err error) {
	return nil, 0, nil
}

func (g *gateway) RenewTokensHTDev(ctx context.Context, request *RenewTokensRequest) (resp *RenewTokensResponse, err error) {
	return nil, nil
}

func (g *gateway) CancelTokens(ctx context.Context, request *CancelTokensRequest) (resp *CancelTokensResponse, err error) {
	return nil, nil
}
