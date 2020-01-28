package bot

type server struct {
	ID     string
	values map[string]map[string]string
	users  map[string]*user
	roles  map[string]*role
	prefix string
}
