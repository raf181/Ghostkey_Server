# Use the official Go image as the base image
FROM golang:1.21-alpine
# Set the working directory inside the container
WORKDIR /app
# Install required system dependencies
RUN apk add --no-cache gcc musl-dev
# Copy go mod files
COPY go.mod go.sum ./
# Download dependencies
RUN go mod download
# Copy the source code
COPY . .
# Build the application
RUN go build -o main .
# Expose port 5000
EXPOSE 5000
# Command to run the application
CMD ["./main"]
