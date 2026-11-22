package domain

import "errors"

var (
	ErrNotFound        = errors.New("not found")
	ErrForbidden       = errors.New("forbidden")
	ErrInvalidArgument = errors.New("invalid argument")
	ErrQueueFull       = errors.New("processing queue is full, try again later")
	ErrAlreadyExists   = errors.New("already exists")
)
