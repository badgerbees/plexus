package ingest

import (
	"plexus/internal/types"
)

type Reader interface {
	Read() ([]types.Document, error)
}
