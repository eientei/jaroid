package bot

type user struct {
	ID    string
	roles map[string]*role
}
