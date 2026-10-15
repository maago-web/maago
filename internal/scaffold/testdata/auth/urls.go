package auth

import "github.com/maago-web/maago"

var Routes = maago.Routes{
	maago.GET("/login", "login", Login),
	maago.POST("/login", "login_submit", LoginSubmit, maago.NoTx),
	maago.POST("/logout", "logout", Logout),
	maago.GET("/signup", "signup", Signup),
	maago.POST("/signup", "signup_submit", SignupSubmit, maago.NoTx),
	maago.GET("/verify", "verify", Verify),
	maago.POST("/verify", "verify_submit", VerifySubmit, maago.NoTx),
	maago.GET("/verify/resend", "verify_resend", VerifyResend),
	maago.POST("/verify/resend", "verify_resend_submit", VerifyResendSubmit, maago.NoTx),
	maago.GET("/password", "password_change", PasswordChange),
	maago.POST("/password", "password_change_submit", PasswordChangeSubmit, maago.NoTx),
	maago.GET("/password/reset", "password_reset", PasswordReset),
	maago.POST("/password/reset", "password_reset_submit", PasswordResetSubmit, maago.NoTx),
	maago.GET("/password/reset/confirm", "password_reset_confirm", PasswordResetConfirm),
	maago.POST("/password/reset/confirm", "password_reset_confirm_submit", PasswordResetConfirmSubmit, maago.NoTx),
	maago.GET("/tokens", "api_tokens", APITokens),
	maago.POST("/tokens", "api_tokens_submit", APITokensSubmit, maago.NoTx),
	maago.POST("/tokens/{id}/delete", "api_token_delete", APITokenDelete),
	maago.GET("/account/close", "account_close", AccountClose),
	maago.POST("/account/close", "account_close_submit", AccountCloseSubmit, maago.NoTx),
}
