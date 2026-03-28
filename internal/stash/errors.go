package stash

import "errors"

var (
	// ErrListNotFound indicates the list doesn't exist
	ErrListNotFound = errors.New("list not found")

	// ErrURLNotFound indicates the URL doesn't exist
	ErrURLNotFound = errors.New("url not found")

	// ErrListURLNotFound indicates the list-URL association doesn't exist
	ErrListURLNotFound = errors.New("url not in list")

	// ErrURLAlreadyInList indicates the URL is already in the list
	ErrURLAlreadyInList = errors.New("url already in list")

	// ErrCannotDeleteDefaultList indicates an attempt to delete the default list
	ErrCannotDeleteDefaultList = errors.New("cannot delete default list")
)
