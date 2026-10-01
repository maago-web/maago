package site

import "github.com/maago-web/maago"

var Routes = maago.Routes{
	maago.GET("/posts/{id}", "post_detail", PostDetail),
	maago.GET("/pages/{n}", "page", Page),
	maago.GET("/list", "post_list", PostList),
	maago.GET("/list/rows", "post_rows", PostList, maago.Block("rows")),
	maago.POST("/posts", "post_create", PostCreate),
	maago.GET("/api/posts/{id}", "api_post", APIPost, maago.API),
	maago.GET("/broken", "broken", Broken),
}
