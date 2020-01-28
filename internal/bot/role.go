package bot

type role struct {
	ID    string
	users map[string]*user
}
