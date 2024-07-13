### Updated Documentation

# Ghostkey_Server

C2 server for the Ghostkey project.

## Usage

1. Set the environment variable:
    - `SECRET_KEY` as an environment variable or in your deployment environment.

```sh
export SECRET_KEY=your_secret_key
go run main.go models.go routes.go
```

## Routes

### 1. Register User

To register a new user, ensure you provide the `SECRET_KEY` along with the username and password.

```sh
curl -X POST http://localhost:5000/register_user -H "Content-Type: application/x-www-form-urlencoded" -d "username=new_user&password=password123&secret_key=your_secret_key"
```

### 2. Login

To log in a user:

```sh
curl -X POST http://localhost:5000/register_user -H "Content-Type: application/x-www-form-urlencoded" -d "username=new_user&password=password123"
```

### 3. Logout

To log out the current user:

```sh
curl -X POST http://localhost:5000/logout -H "Content-Type: application/x-www-form-urlencoded"
```

### 4. Register ESP Device

To register a new ESP device, you need to be logged in:

```sh
curl -X POST http://localhost:5000/register_device -H "Content-Type: application/x-www-form-urlencoded" -d "esp_id=esp32_1&esp_secret_key=your_esp_secret_key"
```

### 5. Send Command

To send a command to an ESP device, you need to be logged in:

```sh
curl -X POST http://localhost:5000/command -H "Content-Type: application/x-www-form-urlencoded" -d "esp_id=esp32_1&command=your_command_here"
```

### 6. Get Command

To get a command for a specific ESP device, no login is required:

```sh
curl -X GET http://localhost:5000/get_command -H "Content-Type: application/x-www-form-urlencoded" -d "esp_id=esp32_1&esp_secret_key=your_esp_secret_key"
```

### Notes

- Make sure to replace `your_secret_key` in the `register_user` request with the actual secret key defined in your environment variables.
- Replace `"esp32_1"` and `"esp_secret_key_123"` with actual values relevant to your ESP devices.
- For the `/register_device` and `/command` routes, you must be logged in. To achieve this using `curl`, you may need to handle cookies or session tokens, which are generally stored in a file and passed in subsequent requests. Here’s an example of how to manage login sessions with `curl`:

#### Example for Handling Login Sessions

1. **Login and Save Session:**

```sh
curl -c cookies.txt -X POST http://localhost:5000/login -H "Content-Type: application/x-www-form-urlencoded" -d "username=new_user&password=password123"
```

2. **Use Saved Session to Register Device:**

```sh
curl -b cookies.txt -X POST http://localhost:5000/register_device -H "Content-Type: application/x-www-form-urlencoded" -d "esp_id=esp32_1&esp_secret_key=your_esp_secret_key"
```

3. **Use Saved Session to Send Command:**

```sh
curl -b cookies.txt -X POST http://localhost:5000/command -H "Content-Type: application/x-www-form-urlencoded" -d "esp_id=esp32_1&command=your_command_here"
```

This approach ensures that the session information (cookies) is preserved between requests, simulating a logged-in state.