package obinary

import (
	"gopkg.in/cetrea/orientgo.v2/obinary/rw"
)

func ReadErrorResponse(r *rw.Reader) (serverException error) {
	return readErrorResponse(r, CurrentProtoVersion)
}
