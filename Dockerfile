FROM golang:1.24-alpine AS builder

# copy the source code
COPY go.mod /build_dir/go.mod
COPY go.sum /build_dir/go.sum
COPY main.go /build_dir/main.go
COPY main_test.go /build_dir/main_test.go
COPY pkg /build_dir/pkg
COPY cmd /build_dir/cmd

# set the working directory
WORKDIR /build_dir

# build the binary
RUN go build -o gocc .


FROM alpine:3

# Install bash
RUN apk add --no-cache bash

# Install adduser
RUN apk add --no-cache shadow

# Install curl
RUN apk add --no-cache curl

# Install nslookup and dig
RUN apk add --no-cache bind-tools

# Create a non-root user to run as, using a predefined uid and gid
RUN addgroup -g 1000 -S gocc && adduser -u 1000 -S gocc -G gocc

# set the user
USER gocc

# set the working directory
WORKDIR /home/gocc

# Add user's home to the PATH
ENV PATH=/home/gocc:$PATH

# Default port
EXPOSE 8080

# Copy the binary
COPY --from=builder /build_dir/gocc /home/gocc/gocc

# Set the entrypoint as the binary
ENTRYPOINT ["gocc"]
