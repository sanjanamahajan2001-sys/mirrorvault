package credentials

type AuthContext struct {
	Passwords map[string]string // engine -> password
}

func NewContext() *AuthContext {
	return &AuthContext{
		Passwords: make(map[string]string),
	}
}

func (c *AuthContext) Set(engine, password string) {
	c.Passwords[engine] = password
}

func (c *AuthContext) Get(engine string) (string, bool) {
	pwd, ok := c.Passwords[engine]
	return pwd, ok
}
