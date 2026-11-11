//go:build cgo && (darwin || linux)

package signals

// cgo is needed because LibGodot installs its handlers with sigaction in C, past Go's os/signal: only
// saving and restoring the whole sigaction from C gives Go its handlers back. LibGodot itself needs cgo,
// so this adds no new requirement.
//
// graphics.gd has no hook between LibGodot installing its SIGINT handler and the end of loading, so while
// the engine loads a guard thread checks the handlers every 0.1 ms and puts Go's back as soon as one is
// replaced. A signal is lost only if it arrives within that 0.1 ms.

/*
#include <pthread.h>
#include <signal.h>
#include <stdatomic.h>
#include <time.h>

static const int goSigs[] = {SIGINT, SIGTERM, SIGQUIT};
enum { nGoSigs = sizeof(goSigs) / sizeof(goSigs[0]) };
static struct sigaction goActs[nGoSigs];
static atomic_int guarding;
static pthread_t guard;

static int isGoHandler(int i) {
	struct sigaction cur;
	return sigaction(goSigs[i], NULL, &cur) == 0 && cur.sa_handler == goActs[i].sa_handler;
}

static void restoreReplaced(void) {
	for (int i = 0; i < nGoSigs; i++) {
		if (!isGoHandler(i)) sigaction(goSigs[i], &goActs[i], NULL);
	}
}

static void *guardLoop(void *arg) {
	struct timespec pause = {0, 100000};
	while (atomic_load(&guarding)) {
		restoreReplaced();
		nanosleep(&pause, NULL);
	}
	return NULL;
}

static void saveGoSignals(void) {
	for (int i = 0; i < nGoSigs; i++) sigaction(goSigs[i], NULL, &goActs[i]);
	atomic_store(&guarding, 1);
	if (pthread_create(&guard, NULL, guardLoop, NULL) != 0) atomic_store(&guarding, 0);
}

static void restoreGoSignals(void) {
	if (atomic_exchange(&guarding, 0)) pthread_join(guard, NULL);
	for (int i = 0; i < nGoSigs; i++) sigaction(goSigs[i], &goActs[i], NULL);
}

static int goSigCount(void) { return nGoSigs; }

static int goHandlersInstalled(void) {
	for (int i = 0; i < nGoSigs; i++) {
		if (!isGoHandler(i)) return 0;
	}
	return 1;
}

static void swallow(int sig) {}

static void installSwallower(void) {
	struct sigaction a = {0};
	a.sa_handler = swallow;
	sigaction(SIGINT, &a, NULL);
}
*/
import "C"

import (
	"os"
	"syscall"
)

// Quit are the OS signals the game exits on. The C list goSigs must hold the same signals.
var Quit = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT}

// Save remembers Go's handlers of Quit and, until Restore, puts them back whenever LibGodot replaces one.
func Save() { C.saveGoSignals() }

// Restore stops guarding the handlers and hands Quit back to Go after the engine has loaded.
func Restore() { C.restoreGoSignals() }

// cSignalCount returns the number of signals in the C list goSigs, to check it against Quit.
func cSignalCount() int { return int(C.goSigCount()) }

// goHandlersInstalled reports whether Go's handlers saved by Save are installed for all of Quit.
func goHandlersInstalled() bool { return C.goHandlersInstalled() == 1 }

// installSwallower replaces the SIGINT handler with one that ignores the signal, as LibGodot does on macOS.
func installSwallower() { C.installSwallower() }
