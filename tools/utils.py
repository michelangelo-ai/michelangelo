import sys
import os


def include_python_dir():
    """
    Adds the 'python' directory to the system path,
    allowing Python-based tools to import Michelangelo SDK code using the syntax "from michelangelo import ..."
    """
    root = os.environ["WORKSPACE_ROOT"]
    py_root = os.path.join(root, "python")
    sys.path.append(py_root)
