from flask import Flask
from flask_sqlalchemy import SQLAlchemy
from flask_login import LoginManager
from flask_migrate import Migrate
from routes import blueprint_name
from models import db, User

app = Flask(__name__)
app.config['SECRET_KEY'] = 'your_secret_key'  # Replace with a strong secret key
app.config['SQLALCHEMY_DATABASE_URI'] = 'sqlite:///c2_framework.db'
app.config['SQLALCHEMY_BINDS'] = {
    'backup':'sqlite:///c2_framework_backup.db',
    'backup2':'sqlite:///c2_framework_backup2.db',
    'backup3':'sqlite:///c2_framework_backup3.db'
}
app.config['SQLALCHEMY_TRACK_MODIFICATIONS'] = False

# Initialize db with the app
db.init_app(app)

# Migrate for database schema changes
migrate = Migrate(app, db)

# Create all tables based on your models
with app.app_context():
    db.create_all()

login_manager = LoginManager()
login_manager.init_app(app)
login_manager.login_view = 'blueprint_name.login'

@login_manager.user_loader
def load_user(user_id):
    return User.query.get(int(user_id))

# Register blueprint
app.register_blueprint(blueprint_name)

if __name__ == '__main__':
    app.run(host="0.0.0.0")
