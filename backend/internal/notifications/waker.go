package notifications

// Waker lets the code that queues a notification tell the dispatcher "there is something new" so it sends right
// away instead of waiting for its next periodic look.
//
// It is a signal, not a queue: the channel holds at most one pending wake-up, so any number of Wake calls made
// while the dispatcher is busy collapse into a single extra run. The periodic look stays as the safety net (and
// is what retries failed sends when they fall due), so a lost wake-up only costs a delay, never an email.
//
// A nil *Waker is valid and does nothing, which is what tests and commands that send no email use.
type Waker struct{ ch chan struct{} }

// NewWaker returns a Waker. Pass it to both the Writer (which calls Wake) and the Dispatcher (which waits on it).
func NewWaker() *Waker { return &Waker{ch: make(chan struct{}, 1)} }

// Wake asks the dispatcher to run now. It never blocks.
func (w *Waker) Wake() {
	if w == nil {
		return
	}
	select {
	case w.ch <- struct{}{}:
	default: // a wake-up is already pending
	}
}

// signal is the channel the dispatcher selects on. For a nil Waker it is a nil channel, which blocks forever.
func (w *Waker) signal() <-chan struct{} {
	if w == nil {
		return nil
	}
	return w.ch
}
