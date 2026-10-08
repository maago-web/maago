package auth

import "github.com/maago-web/maago"

var Routes = maago.Routes{
	maago.GET("/login", "login", Login),
	maago.POST("/login", "login_submit", Login),
	maago.POST("/logout", "logout", Logout),
}
