package core

import (
	"context"

	"github.com/google/uuid"
)

type Context = context.Context

type UUID = uuid.UUID

var NewUUID = uuid.New

type Generation = int64
