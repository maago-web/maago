package site

import (
	"context"

	"github.com/maago-web/maago"
)

type PostDetailIn struct {
	ID int64 `maago:"path=id"`
}

type PostPage struct {
	Post Post
}

func PostDetail(ctx context.Context, in PostDetailIn) (PostPage, error) {
	p, err := GetPost(ctx, maago.DBFrom(ctx), in.ID)
	return PostPage{Post: p}, err
}

type PageIn struct {
	N int32 `maago:"path=n"`
}

type NumberPage struct {
	N int32
}

func Page(ctx context.Context, in PageIn) (NumberPage, error) { return NumberPage{in.N}, nil }

type ListIn struct {
	Page  int32 `maago:"query=page"`
	Draft *bool `maago:"query=draft"`
}

type ListPage struct {
	Page  int32
	Draft string
}

func PostList(ctx context.Context, in ListIn) (ListPage, error) {
	draft := "unset"
	if in.Draft != nil {
		draft = map[bool]string{true: "yes", false: "no"}[*in.Draft]
	}
	return ListPage{Page: in.Page, Draft: draft}, nil
}

func PostCreate(ctx context.Context, in struct{}) (maago.Result[PostPage], error) {
	p, err := InsertPost(ctx, maago.DBFrom(ctx), NewPostInput{Title: "new"})
	if err != nil {
		return maago.Result[PostPage]{}, err
	}
	return maago.SeeOther[PostPage](PostDetailURL(p.ID)), nil
}

func APIPost(ctx context.Context, in PostDetailIn) (Post, error) {
	return GetPost(ctx, maago.DBFrom(ctx), in.ID)
}

type BrokenPage struct {
	Post *Post // nil: the template fails on it
}

func Broken(ctx context.Context, in struct{}) (BrokenPage, error) { return BrokenPage{}, nil }
