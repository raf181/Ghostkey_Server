# routes.py
from flask import request, jsonify
from flask_sqlalchemy import SQLAlchemy
from flask_login import LoginManager, UserMixin, login_user, login_required, logout_user, current_user
from werkzeug.security import generate_password_hash, check_password_hash

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
        secret_key = db.Column(db.String(120), nullable=False)

    @login_manager.user_loader
    def load_user(user_id):
        return User.query.get(int(user_id))



    @app.route('/register_user', methods=['POST'])
    def register_user():
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



    @app.route('/register', methods=['POST'])
    @login_required
    def register():
        esp_id = request.json.get('esp_id')
        secret_key = request.json.get('secret_key')

        if not esp_id or not secret_key:
            return jsonify({'message': 'ESP ID and secret key are required'}), 400

        return jsonify({'message': 'ESP32 registered successfully', 'esp_id': esp_id, 'secret_key': secret_key})



    @app.route('/command', methods=['POST'])
    @login_required
    def command():
        esp_id = request.json.get('esp_id')
        command_text = request.json.get('command')
        secret_key = request.json.get('secret_key')

        if not esp_id or not command_text or not secret_key:
            return jsonify({'message': 'ESP ID, command, and secret key are required'}), 400

        # Validate the ESP ID and secret key combination
        command_exists = Command.query.filter_by(esp_id=esp_id, secret_key=secret_key).first()
        if not command_exists:
            return jsonify({'message': 'Invalid ESP ID or secret key'}), 400

        new_command = Command(esp_id=esp_id, command=command_text, secret_key=secret_key)
        try:
            db.session.add(new_command)
            db.session.commit()
            return jsonify({'message': 'Command added successfully'})
        except Exception as e:
            db.session.rollback()
            return jsonify({'message': f'An error occurred: {e}'}), 500



    @app.route('/get_command', methods=['GET'])
    @login_required
    def get_command():
        esp_id = request.args.get('esp_id')
        secret_key = request.args.get('secret_key')

        if not esp_id or not secret_key:
            return jsonify({'message': 'ESP ID and secret key are required'}), 400

        # Validate the ESP ID and secret key combination
        command = Command.query.filter_by(esp_id=esp_id, secret_key=secret_key).order_by(Command.id).first()
        if command:
            try:
                db.session.delete(command)
                db.session.commit()
                return jsonify({'command': command.command})
            except Exception as e:
                db.session.rollback()
                return jsonify({'message': f'An error occurred: {e}'}), 500

        # If no command is found, return 'command: None'
        return jsonify({'command': None})
