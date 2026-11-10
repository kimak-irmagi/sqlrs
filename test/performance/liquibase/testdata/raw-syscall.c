/* UR06: Actions-only native tracer fixture. No database, network or stdout.
 * The ready/release handshake lets the test verify the exact live child first.
 * ENOSYS is checked independently of the observer's trace decoder. */
#define _GNU_SOURCE
#include <errno.h>
#include <stdio.h>
#include <sys/syscall.h>
#include <unistd.h>

int main(int argc, char **argv) {
    if (argc != 3) return 2;
    FILE *ready = fopen(argv[1], "wx");
    if (!ready) return 3;
    if (fprintf(ready, "%ld", (long)getpid()) < 0 || fclose(ready)) return 4;
    unsigned int tries = 0;
    while (access(argv[2], F_OK)) {
        if (++tries >= 500) return 5;
        usleep(10000);
    }
    errno = 0;
    long result = syscall(0x3fffffffL, 0ULL, 1ULL, ~0ULL, 2ULL, 0ULL, 3ULL);
    return (result == -1 && errno == ENOSYS) ? 0 : 6;
}
