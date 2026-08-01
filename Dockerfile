# Stage 1: build
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache make
WORKDIR /app
COPY . .
RUN make build

# Stage 2: test
FROM node:22-alpine AS tester
COPY --from=builder /app/build/pkgfort /usr/local/bin/pkgfort
COPY --from=builder /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}"
ENV PKGFORT_VERBOSE=1
RUN echo 'export PATH="/usr/local/go/bin:$PATH"' > /etc/profile.d/golang.sh
COPY --from=builder /app/test /app/test
RUN pkgfort install && echo '. ~/.pkgfort/init.sh' >> ~/.profile
WORKDIR /app/test
RUN chmod +x ./run_test.sh && sh -l ./run_test.sh
