package auth

import "github.com/maago-web/maago"

var Routes = maago.Routes{
	maago.GET("/login", "login", Login),
	maago.POST("/login", "login_submit", Login),
	maago.POST("/logout", "logout", Logout),
	maago.GET("/signup", "signup", Signup),
	maago.POST("/signup", "signup_submit", Signup),
	maago.GET("/verify", "verify", Verify),
	maago.POST("/verify", "verify_submit", Verify),
	maago.GET("/verify/resend", "verify_resend", VerifyResend),
	maago.POST("/verify/resend", "verify_resend_submit", VerifyResend),
	maago.GET("/password", "password_change", PasswordChange, maago.LoginRequired),
	maago.POST("/password", "password_change_submit", PasswordChange, maago.LoginRequired),
	maago.GET("/password/reset", "password_reset", PasswordReset),
	maago.POST("/password/reset", "password_reset_submit", PasswordReset),
	maago.GET("/password/reset/confirm", "password_reset_confirm", PasswordResetConfirm),
	maago.POST("/password/reset/confirm", "password_reset_confirm_submit", PasswordResetConfirm),
	maago.GET("/tokens", "api_tokens", APITokens, maago.LoginRequired),
	maago.POST("/tokens", "api_tokens_submit", APITokens, maago.LoginRequired),
	maago.POST("/tokens/{id}/delete", "api_token_delete", APITokenDelete, maago.LoginRequired),
	maago.GET("/account/close", "account_close", AccountClose, maago.LoginRequired),
	maago.POST("/account/close", "account_close_submit", AccountClose, maago.LoginRequired),
}
