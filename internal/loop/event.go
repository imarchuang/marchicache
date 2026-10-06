package loop

type Event struct {
	Fd    int
	Read  bool
	Write bool
}
