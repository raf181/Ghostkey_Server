![Ghostkey](https://github.com/raf181/Ghostkey/blob/main/wiki/source/Untitled.webp)

# Ghostkey_Server

C2 server for the Ghostkey project.

> [!warning] 
> **Warning** these is only a proof of concept.
>
> The project is still in development and is not ready for real use. The project is not responsible for any damage caused by the use of this tool. Use it at your own risk.

## Usage

1. Set the environment variable:
    - `SECRET_KEY` as an environment variable or in your deployment environment.

```sh
export SECRET_KEY=your_secret_key
go run main.go models.go routes.go
```

you might get these error

```sh
go run main.go models.go routes.go

2024/10/06 01:54:14 /home/anoam/github/Ghostkey_Server/main.go:27
[error] failed to initialize database, got error Binary was compiled with 'CGO_ENABLED=0', go-sqlite3 requires cgo to work. This is a stub
2024/10/06 01:54:14 Failed to connect to database: Binary was compiled with 'CGO_ENABLED=0', go-sqlite3 requires cgo to work. This is a stub
exit status 1
```

to fix it run these `sudo apt install build-essential`

## Routes

### 1. Register User

To register a new user, ensure you provide the `SECRET_KEY` along with the username and password.

```sh
curl --location 'http://localhost:5000/register_user' \
--header 'Content-Type: application/x-www-form-urlencoded' \
--data-urlencode 'username=new_user' \
--data-urlencode 'password=password123' \
--data-urlencode 'secret_key=your_secret_key'
```

### 2. Login

To log in a user:

```sh
curl --location 'http://localhost:5000/login' \
--header 'Content-Type: application/x-www-form-urlencoded' \
--data-urlencode 'username=new_user' \
--data-urlencode 'password=password123' \
--cookie-jar cookies.txt
```

### 3. Logout

To log out the current user (requires authentication):

```sh
curl --location 'http://localhost:5000/logout' \
--cookie cookies.txt
```

### 4. Register ESP Device

To register a new ESP device (requires authentication):

```sh
curl --location 'http://localhost:5000/register_device' \
--header 'Content-Type: application/x-www-form-urlencoded' \
--cookie cookies.txt \
--data-urlencode 'esp_id=esp32_1' \
--data-urlencode 'esp_secret_key=your_esp_secret_key'
```

### 5. Send Command

To send a command to an ESP device (requires authentication):

```sh
curl --location 'http://localhost:5000/command' \
--header 'Content-Type: application/x-www-form-urlencoded' \
--cookie cookies.txt \
--data-urlencode 'esp_id=esp32_1' \
--data-urlencode 'command=your_command_here'
```

### 6. Get Command

To get a command for a specific ESP device (requires ESP authentication):

```sh
curl --location 'http://localhost:5000/get_command' \
--header 'Content-Type: application/x-www-form-urlencoded' \
--data-urlencode 'esp_id=esp32_1' \
--data-urlencode 'esp_secret_key=your_esp_secret_key'
```

### Notes

- Make sure to replace `your_secret_key` with the actual secret key defined in your environment variables.
- Replace `esp32_1` and `your_esp_secret_key` with actual values relevant to your ESP devices.
- All authenticated routes require a valid session cookie. The examples above use cookie-based authentication:
  1. First login using the `/login` endpoint which saves the session cookie to `cookies.txt`
  2. Then use this cookie file with `--cookie cookies.txt` for subsequent authenticated requests

### Register Mailer

To register a mailer (requires authentication):

```sh
curl --location 'http://localhost:5000/register_mailer' \
--header 'Content-Type: application/x-www-form-urlencoded' \
--cookie cookies.txt \
--data-urlencode 'esp_id=your_esp_id_here' \
--data-urlencode 'delivery_key=your_delivery_key_here' \
--data-urlencode 'encryption_password=YourEncryptionPassword'
```

## Docker

Run the server using Docker:

```sh
docker-compose up --build
```

# Collaboration
If you want to collaborate with the project or make your own version of the Ghostkey, feel free to do so. I only ask that you share with me your version of the project so I can learn from it and find ways to improve the Ghostkey.

The project is open source and is under the [GPL-3.0 license](https://github.com/raf181/Ghostkey/blob/main/LICENSE), and I have no intention of changing that. Since it has the following conditions:

| Permissions                                                                                | Limitations               | Conditions                                                                                   |
| ------------------------------------------------------------------------------------------ | ------------------------- | -------------------------------------------------------------------------------------------- |
| Commercial use ✔️<br>Modification ✔️<br>Distribution ✔️<br>Patent use ✔️<br>Private use ✔️ | Liability ❌<br>Warranty ❌ | License and copyright notice ℹ️<br>State changes ℹ️<br>Disclose source ℹ️<br>Same license ℹ️ |
