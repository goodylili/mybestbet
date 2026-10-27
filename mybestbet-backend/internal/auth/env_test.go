package auth

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/config"
)

func TestNewFailsClosedOutsideLocalEnvs(t *testing.T) {
	t.Parallel()
	secrets := config.Auth{
		JWTPrivateKey: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		CodePepper:    "pepper-pepper-pepper-pepper-pepper",
		ResendAPIKey:  "re_key",
		EmailFrom:     "codes@odds.test",
		WebOrigin:     "https://odds.test",
	}
	tests := []struct {
		name  string
		env   string
		auth  config.Auth
		valid bool
	}{
		{name: "dev falls back to ephemeral secrets", env: "dev", valid: true},
		{name: "test falls back to ephemeral secrets", env: "test", valid: true},
		{name: "empty env requires secrets", env: ""},
		{name: "prod requires secrets", env: "prod"},
		{name: "staging requires secrets", env: "staging"},
		{name: "typo requires secrets", env: "Dev"},
		{name: "prod with every secret", env: "prod", auth: secrets, valid: true},
		{name: "prod without a web origin", env: "prod", auth: with(secrets, func(a *config.Auth) { a.WebOrigin = "" })},
		{name: "prod without resend", env: "prod", auth: with(secrets, func(a *config.Auth) { a.ResendAPIKey = "" })},
		{name: "prod without a pepper", env: "prod", auth: with(secrets, func(a *config.Auth) { a.CodePepper = "" })},
		{name: "prod without a jwt key", env: "prod", auth: with(secrets, func(a *config.Auth) { a.JWTPrivateKey = "" })},
		{name: "short pepper", env: "dev", auth: config.Auth{CodePepper: "short"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(Deps{Config: config.Config{Env: tt.env, Auth: tt.auth}})
			if tt.valid {
				require.NoError(t, err)
				return
			}
			assert.Error(t, err)
		})
	}
}
