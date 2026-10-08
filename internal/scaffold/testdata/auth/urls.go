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
}
