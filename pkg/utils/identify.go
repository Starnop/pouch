package utils

import (
	"context"
	"net/http"
	"strings"
)

// SetTlsIssuer set issuer name of tls to context
func SetTlsIssuer(ctx context.Context, issuer string) context.Context {
	return context.WithValue(ctx, "pouch.server.tls.issuer", issuer)
}

// SetTlsIssuer fetch issuer name from context
func GetTlsIssuer(ctx context.Context) string {
	issuer := ctx.Value("pouch.server.tls.issuer")
	if issuer == nil {
		return ""
	}
	return issuer.(string)
}

// SetTlsCommonName set common name of tls to context
func SetTlsCommonName(ctx context.Context, cn string) context.Context {
	return context.WithValue(ctx, "pouch.server.tls.cn", cn)
}

// GetTlsCommonName fetch common name from context
func GetTlsCommonName(ctx context.Context) string {
	issuer := ctx.Value("pouch.server.tls.cn")
	if issuer == nil {
		return ""
	}
	return issuer.(string)
}

// IsSigma checks tls name and return if this context is owned by sigma
func IsSigma(ctx context.Context, req *http.Request) bool {
	isRemoteSigma := GetTlsIssuer(ctx) == "ali" && strings.Contains(GetTlsCommonName(ctx), "sigma")
	if req == nil {
		return isRemoteSigma
	}
	return isRemoteSigma || strings.Contains(req.Header.Get("User-Agent"), "Docker-Client")
}
