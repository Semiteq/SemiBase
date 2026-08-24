# The image carries the linux/amd64 binary and nothing else. Its only consumer
# copies the binary out (COPY --from=ghcr.io/semiteq/semibase:latest /semibase),
# so a base with a shell, CA certificates or a libc would ship weight nobody
# runs: the module is pure pgx, builds CGO_ENABLED=0 and speaks no TLS on the
# bench. The ENTRYPOINT costs one line and makes `docker run <image> version`
# a valid smoke check.
FROM scratch
COPY semibase /semibase
ENTRYPOINT ["/semibase"]
