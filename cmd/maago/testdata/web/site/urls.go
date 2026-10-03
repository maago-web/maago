package site

import "github.com/maago-web/maago"

var Routes = maago.Routes{
	maago.GET("/posts/{id}", "post_detail", PostDetail),
	maago.GET("/pages/{n}", "page", Page),
	maago.GET("/list", "post_list", PostList),
	maago.GET("/list/rows", "post_rows", PostList, maago.Block("rows")),
	maago.POST("/posts", "post_create", PostCreate),
	maago.GET("/posts/new", "post_new", PostForm),
	maago.POST("/posts/new", "post_new_save", PostForm),
	maago.GET("/api/posts/{id}", "api_post", APIPost, maago.API),
	maago.POST("/api/posts", "api_post_create", APIPostCreate, maago.API),
	maago.GET("/broken", "broken", Broken),
	maago.GET("/export/{format}", "post_export", PostExport),
}
