# Stage 1: The build environment
# Use an official Go image to build the application
FROM golang:1.24.5-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy the go.mod and go.sum files to download dependencies
COPY go.mod ./
COPY go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the Go application, creating a static binary
# CGO_ENABLED=0 is important for creating a static binary without C dependencies
# -o /portfolio-app creates the binary at the root named 'portfolio-app'
RUN CGO_ENABLED=0 GOOS=linux go build -o /portfolio-app .

# Stage 2: The production environment
# Use a minimal 'distroless' image which contains only our app and its runtime dependencies.
# It's more secure because it doesn't contain a shell or other programs.
FROM gcr.io/distroless/static-debian11

# Set the working directory
WORKDIR /

# Copy the built binary from the 'builder' stage
COPY --from=builder /portfolio-app /portfolio-app

# Copy templates directory
COPY --from=builder /app/templates /templates

# Set the PORT environment variable that Cloud Run uses
ENV PORT 8080

# The command to run when the container starts
CMD ["/portfolio-app"]