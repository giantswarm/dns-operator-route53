package scope

import (
	"sync"

	"github.com/aws/aws-sdk-go/aws/session"
)

var sessionCache sync.Map

type sessionCacheEntry struct {
	session *session.Session
}

func sessionForCluster(id string) (*session.Session, error) {
	if s, ok := sessionCache.Load(id); ok {
		entry := s.(*sessionCacheEntry)
		return entry.session, nil
	}

	ns, err := session.NewSession()
	if err != nil {
		return nil, err
	}

	sessionCache.Store(id, &sessionCacheEntry{
		session: ns,
	})
	return ns, nil
}
