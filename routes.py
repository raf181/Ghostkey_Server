# routes.py
from flask import request, jsonify
from flask_sqlalchemy import SQLAlchemy
from flask_login import LoginManager, UserMixin, login_user, login_required, logout_user, current_user
from werkzeug.security import generate_password_hash, check_password_hash
from datetime import datetime

def register_routes(app, db):
    # Initialize Flask-Login
    login_manager = LoginManager()
    login_manager.init_app(app)
    login_manager.login_view = 'login'

    # Define the User model
    class User(UserMixin, db.Model):
        id = db.Column(db.Integer, primary_key=True)
        username = db.Column(db.String(80), unique=True, nullable=False)
        password_hash = db.Column(db.String(120), nullable=False)

        def set_password(self, password):
            self.password_hash = generate_password_hash(password)

        def check_password(self, password):
            return check_password_hash(self.password_hash, password)

    # Define the Command model
    class Command(db.Model):
        id = db.Column(db.Integer, primary_key=True)
        esp_id = db.Column(db.String(80), nullable=False)
        command = db.Column(db.String(120), nullable=False)

    # Define the ESPDevice model
    class ESPDevice(db.Model):
        id = db.Column(db.Integer, primary_key=True)
        esp_id = db.Column(db.String(80), unique=True, nullable=False)
        esp_secret_key = db.Column(db.String(120), nullable=False)
        last_request_time = db.Column(db.DateTime, nullable=True)

    @login_manager.user_loader
    def load_user(user_id):
        return User.query.get(int(user_id))

    @app.route('/register_user', methods=['POST'])
    def register_user():
        secret_key = request.json.get('secret_key')
        if secret_key != app.config['SECRET_KEY']:
            return jsonify({'message': 'Invalid secret key'}), 403

        username = request.json.get('username')
        password = request.json.get('password')
        if not username or not password:
            return jsonify({'message': 'Username and password are required'}), 400

        if User.query.filter_by(username=username).first():
            return jsonify({'message': 'Username already exists'}), 400

        new_user = User(username=username)
        new_user.set_password(password)
        db.session.add(new_user)
        db.session.commit()
        return jsonify({'message': 'User registered successfully'})

    @app.route('/login', methods=['POST'])
    def login():
        username = request.json.get('username')
        password = request.json.get('password')
        if not username or not password:
            return jsonify({'message': 'Username and password are required'}), 400

        user = User.query.filter_by(username=username).first()
        if user is None or not user.check_password(password):
            return jsonify({'message': 'Invalid username or password'}), 400

        login_user(user)
        return jsonify({'message': 'Logged in successfully'})

    @app.route('/logout', methods=['POST'])
    @login_required
    def logout():
        logout_user()
        return jsonify({'message': 'Logged out successfully'})

    @app.route('/register_device', methods=['POST'])
    @login_required
    def register_device():
        esp_id = request.json.get('esp_id')
        esp_secret_key = request.json.get('esp_secret_key')

        if not esp_id or not esp_secret_key:
            return jsonify({'message': 'ESP ID and secret key are required'}), 400

        if ESPDevice.query.filter_by(esp_id=esp_id).first():
            return jsonify({'message': 'ESP ID already exists'}), 400

        new_device = ESPDevice(esp_id=esp_id, esp_secret_key=esp_secret_key)
        db.session.add(new_device)
        db.session.commit()

        return jsonify({'message': 'ESP32 registered successfully', 'esp_id': esp_id})

    @app.route('/command', methods=['POST'])
    @login_required
    def command():
        esp_id = request.json.get('esp_id')
        command_text = request.json.get('command')

        if not esp_id or not command_text:
            return jsonify({'message': 'ESP ID and command are required'}), 400

        # Validate the ESP ID
        device_exists = ESPDevice.query.filter_by(esp_id=esp_id).first()
        if not device_exists:
            return jsonify({'message': 'Invalid ESP ID'}), 400

        new_command = Command(esp_id=esp_id, command=command_text)
        try:
            db.session.add(new_command)
            db.session.commit()
            return jsonify({'message': 'Command added successfully'})
        except Exception as e:
            db.session.rollback()
            return jsonify({'message': f'An error occurred: {e}'}), 500

    @app.route('/get_command', methods=['GET'])
    def get_command():
        esp_id = request.args.get('esp_id')
        esp_secret_key = request.args.get('esp_secret_key')

        if not esp_id or not esp_secret_key:
            return jsonify({'message': 'ESP ID and secret key are required'}), 400

        # Validate the ESP ID and secret key combination
        device = ESPDevice.query.filter_by(esp_id=esp_id, esp_secret_key=esp_secret_key).first()
        if not device:
            return jsonify({'message': 'Invalid ESP ID or secret key'}), 400

        command = Command.query.filter_by(esp_id=esp_id).order_by(Command.id).first()
        if command:
            try:
                # Update the last request time
                device.last_request_time = datetime.utcnow()
                db.session.add(device)
                db.session.delete(command)
                db.session.commit()
                return jsonify({'command': command.command})
            except Exception as e:
                db.session.rollback()
                return jsonify({'message': f'An error occurred: {e}'}), 500

        # If no command is found, return 'command: None'
        return jsonify({'command': None})

    # New route to remove a specific command
    @app.route('/remove_command', methods=['POST'])
    @login_required
    def remove_command():
        command_id = request.json.get('command_id')

        if not command_id:
            return jsonify({'message': 'Command ID is required'}), 400

        command = Command.query.get(command_id)
        if command:
            try:
                db.session.delete(command)
                db.session.commit()
                return jsonify({'message': 'Command removed successfully'})
            except Exception as e:
                db.session.rollback()
                return jsonify({'message': f'An error occurred: {e}'}), 500

        return jsonify({'message': 'Command not found'}), 404

    # New route to get all commands for a specific ESP ID
    @app.route('/get_all_commands', methods=['GET'])
    @login_required
    def get_all_commands():
        esp_id = request.args.get('esp_id')

        if not esp_id:
            return jsonify({'message': 'ESP ID is required'}), 400

        commands = Command.query.filter_by(esp_id=esp_id).order_by(Command.id).all()
        commands_list = [{'id': cmd.id, 'command': cmd.command} for cmd in commands]

        return jsonify({'commands': commands_list})

    # New route to get the last request time for a specific ESP ID
    @app.route('/last_request_time', methods=['GET'])
    @login_required
    def last_request_time():
        esp_id = request.args.get('esp_id')

        if not esp_id:
            return jsonify({'message': 'ESP ID is required'}), 400

        device = ESPDevice.query.filter_by(esp_id=esp_id).first()
        if device:
            return jsonify({'esp_id': esp_id, 'last_request_time': device.last_request_time})

        return jsonify({'message': 'ESP ID not found'}), 404
