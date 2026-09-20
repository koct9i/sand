package journal

type journalWriter interface {
	
}

type journal[W journalWriter] struct {
	writer W
}
