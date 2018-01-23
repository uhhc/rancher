package authn

import (
	"sync"

	"github.com/pkg/errors"
	"github.com/rancher/norman/httperror"
	"github.com/rancher/norman/types"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/client/management/v3"
	"github.com/rancher/types/config"
	"golang.org/x/crypto/bcrypt"
	"k8s.io/client-go/tools/cache"
)

const userByUsernameIndex = "auth.management.cattle.io/user-by-username"

type userStore struct {
	types.Store
	mu          sync.Mutex
	userIndexer cache.Indexer
}

func SetUserStore(schema *types.Schema, mgmt *config.ManagementContext) {
	userInformer := mgmt.Management.Users("").Controller().Informer()
	userIndexers := map[string]cache.IndexFunc{
		userByUsernameIndex: userByUsername,
	}
	userInformer.AddIndexers(userIndexers)

	schema.Store = &userStore{
		Store:       schema.Store,
		mu:          sync.Mutex{},
		userIndexer: userInformer.GetIndexer(),
	}
}

func userByUsername(obj interface{}) ([]string, error) {
	u, ok := obj.(*v3.User)
	if !ok {
		return []string{}, nil
	}

	return []string{u.Username}, nil
}

func hashPassword(data map[string]interface{}) error {
	pass, ok := data[client.UserFieldPassword].(string)
	if !ok {
		return errors.New("password not a string")
	}
	hashed, err := hashPasswordString(pass)
	if err != nil {
		return err
	}
	data[client.UserFieldPassword] = string(hashed)

	return nil
}

func hashPasswordString(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", errors.Wrap(err, "problem encrypting password")
	}
	return string(hash), nil
}

func (s *userStore) Create(apiContext *types.APIContext, schema *types.Schema, data map[string]interface{}) (map[string]interface{}, error) {
	if err := hashPassword(data); err != nil {
		return nil, err
	}

	created, err := s.create(apiContext, schema, data)
	if err != nil {
		return nil, err
	}

	if id, ok := created[types.ResourceFieldID].(string); ok {
		var principalIDs []interface{}
		if pids, ok := created[client.UserFieldPrincipalIDs].([]interface{}); ok {
			principalIDs = pids
		}
		created[client.UserFieldPrincipalIDs] = append(principalIDs, "local://"+id)
		return s.Update(apiContext, schema, created, id)
	}

	return created, err
}

func (s *userStore) create(apiContext *types.APIContext, schema *types.Schema, data map[string]interface{}) (map[string]interface{}, error) {
	username, ok := data[client.UserFieldUsername].(string)
	if !ok {
		return nil, errors.New("invalid username")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	users, err := s.userIndexer.ByIndex(userByUsernameIndex, username)
	if err != nil {
		return nil, err
	}
	if len(users) > 0 {
		return nil, httperror.NewFieldAPIError(httperror.NotUnique, "username", "Username is already in use.")
	}

	return s.Store.Create(apiContext, schema, data)
}

func (s *userStore) List(apiContext *types.APIContext, schema *types.Schema, opt *types.QueryOptions) ([]map[string]interface{}, error) {

	req := apiContext.Request

	schemaData, err := s.Store.List(apiContext, schema, opt)
	if err != nil {
		return nil, err
	}
	userID := req.Header.Get("Impersonate-User")
	if userID != "" {
		for _, data := range schemaData {
			id, ok := data[types.ResourceFieldID].(string)
			if ok {
				if id == userID {
					data["me"] = "true"
				}
			}
		}
	}
	return schemaData, nil
}
