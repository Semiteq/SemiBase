# The image carries the linux/amd64 binary and nothing else. Its only consumer
# copies the binary out (COPY --from=ghcr.io/semiteq/semibase:latest /semibase),
# so a base with a shell, CA certificates or a libc would ship weight nobody
# runs: the module is pure pgx, builds CGO_ENABLED=0 and speaks no TLS on the
# bench. The ENTRYPOINT costs one line and makes `docker run <image> version`
# a valid smoke check.
#
# Build it with `--platform linux/amd64`, the way both workflows do. FROM
# scratch takes the manifest from the builder's own platform, so without the
# flag the manifest states whatever machine ran the build while the payload is
# always the linux/amd64 artifact. A `FROM --platform=` pin does NOT fix that -
# it names the platform of the base stage, and scratch carries no config for
# the output to inherit, so the output platform stays the builder's. The
# release job reads the manifest back and fails if it says anything else.
FROM scratch

# The documented mechanism that links the GHCR package back to this repository.
LABEL org.opencontainers.image.source="https://github.com/Semiteq/SemiBase"

COPY semibase /semibase
ENTRYPOINT ["/semibase"]
