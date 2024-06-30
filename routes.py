from flask import request, jsonify, current_app
from flask_sqlalchemy import SQLAlchemy
from flask_login import LoginManager, UserMixin, login_user, login_required, logout_user, current_user
from werkzeug.security import generate_password_hash, check_password_hash
import logging

def register_routes(app, db):
    # Initialize Flask-Login
    login_manager = LoginManager()
    login_manager.init_app(app)
    login_manager.login_view = 'login'

    # Setup logging
    logger = logging.getLogger(__name__)

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

    @login_manager.user_loader
    def load_user(user_id):
        return User.query.get(int(user_id))

    @app.route('/register_user', methods=['POST'])
    def register_user():
        data = request.get_json()
        username = data.get('username')
        password = data.get('password')
        provided_secret_key = data.get('secret_key')

        if not username or not password or not provided_secret_key:
            return jsonify({'message': 'Username, password, and secret key are required'}), 400

        if provided_secret_key != current_app.config['SECRET_KEY']:
            return jsonify({'message': 'Invalid secret key'}), 403

        if User.query.filter_by(username=username).first():
            return jsonify({'message': 'Username already exists'}), 400

        new_user = User(username=username)
        new_user.set_password(password)
        db.session.add(new_user)
        db.session.commit()
        return jsonify({'message': 'User registered successfully'})

    @app.route('/login', methods=['POST'])
    def login():
        data = request.get_json()
        username = data.get('username')
        password = data.get('password')

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
        data = request.get_json()
        esp_id = data.get('esp_id')
        esp_secret_key = data.get('esp_secret_key')

        if not esp_id or not esp_secret_key:
            return jsonify({'message': 'ESP ID and secret key are required'}), 400

        if ESPDevice.query.filter_by(esp_id=esp_id).first():
            return jsonify({'message': 'ESP ID already exists'}), 400

        new_device = ESPDevice(esp_id=esp_id, esp_secret_key=generate_password_hash(esp_secret_key))
        db.session.add(new_device)
        db.session.commit()

        return jsonify({'message': 'ESP32 registered successfully', 'esp_id': esp_id})

    @app.route('/command', methods=['POST'])
    @login_required
    def command():
        data = request.get_json()
        esp_id = data.get('esp_id')
        command_text = data.get('command')

        if not esp_id or not command_text:
            return jsonify({'message': 'ESP ID and command are required'}), 400

        device = ESPDevice.query.filter_by(esp_id=esp_id).first()
        if not device:
            return jsonify({'message': 'Invalid ESP ID'}), 400

        new_command = Command(esp_id=esp_id, command=command_text)
        try:
            db.session.add(new_command)
            db.session.commit()
            return jsonify({'message': 'Command added successfully'})
        except Exception as e:
            db.session.rollback()
            logger.error(f'Error adding command: {e}')
            return jsonify({'message': 'An error occurred'}), 500

    @app.route('/get_command', methods=['GET'])
    def get_command():
        esp_id = request.args.get('esp_id')
        esp_secret_key = request.args.get('esp_secret_key')

        if not esp_id or not esp_secret_key:
            return jsonify({'message': 'ESP ID and secret key are required'}), 400

        device = ESPDevice.query.filter_by(esp_id=esp_id).first()
        if not device or not check_password_hash(device.esp_secret_key, esp_secret_key):
            return jsonify({'message': 'Invalid ESP ID or secret key'}), 400

        command = Command.query.filter_by(esp_id=esp_id).order_by(Command.id).first()
        if command:
            try:
                db.session.delete(command)
                db.session.commit()
                return jsonify({'command': command.command})
            except Exception as e:
                db.session.rollback()
                logger.error(f'Error deleting command: {e}')
                return jsonify({'message': 'An error occurred'}), 500

        return jsonify({'command': None})
