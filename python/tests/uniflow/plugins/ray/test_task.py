import unittest
import re
from unittest.mock import Mock
from starlark_go import Starlark, configure_starlark

class TestTaskFunction(unittest.TestCase):
    def setUp(self):
        # Initialize the Starlark interpreter
        self.s = Starlark(globals={"load": None})

        print("=============mock callable========")
        # Load the Starlark script dynamically
        with open("michelangelo/uniflow/plugins/ray/task.star") as f:
            self.script = f.read()

        common_scripts = ""
        with open("michelangelo/uniflow/commons.star") as f1:
            common_scripts = f1.read()
        self.script = common_scripts + self.script

        starlark_script = """
def callable_object(arg):
    return "Callable executed with arg: " + str(arg)
ray = {
    "create_cluster": lambda cluster: "Mocked cluster created for: " + str(cluster)
}
os = {
    "environ": {
        "get": lambda key, default="development": "mocked_" + key.lower() if key in ["ENV"] else default.lower()
    }
}
uuid = {
    "uuid4": {
        "hex": "uuid-test"
    }
}
time = {
    "time": lambda: 1700000000,  # Mocked timestamp
    "utc_format_seconds": lambda fmt, sec: "Mocked Time: " + str(sec)
}
progress = {
    "task_state_pending": "TASK_STATE_PENDING",
    "task_state_running": "TASK_STATE_RUNNING",
    "task_state_succeeded": "TASK_STATE_SUCCEEDED",
    "task_state_failed": "TASK_STATE_FAILED",
    "task_state_killed": "TASK_STATE_KILLED",
    "task_state_skipped": "TASK_STATE_SKIPPED",
    "report": lambda state: print("Mocked progress report:", state)
}
hashlib = {
    "blake2b_hex": lambda image, digest_size=16: "mocked_hash_" + str(digest_size)
}
json = {
    "dumps": lambda obj: str(obj)  # Convert objects to a string representation
}
atexit = {
    "register": lambda func: print("Mocked atexit.register:", func),
    "unregister": lambda func: print("Mocked atexit.unregister:", func)
}
COMMONS_ENV = "ENV"
        """
        # Execute the script inside Starlark to define callable_object
        self.exec(starlark_script)

    def exec(self, script):
        # Remove all `load(...)` lines
        script = re.sub(r'load\(.*?\)\n', '', script)
        print("\n=========script start=========\n")
        print(script)
        print("\n=========script end=========\n")
        self.s.exec(script)

    def eval(self, script):
        # Remove all `load(...)` lines
        script = re.sub(r'load\(.*?\)\n', '', script)
        print("\n=========script start=========\n")
        print(script)
        print("\n=========script end=========\n")
        return self.s.eval(script)

    def test_task(self):

        self.exec(self.script)
        # Retrieve the `task` function from the Starlark environment
        task_function = self.s.eval("task")
        self.assertIsNotNone(task_function, "The 'task' function was not found in the Starlark environment.")
        self.assertTrue(callable(task_function), "The 'task' object is not callable.")

        # Call the `task` function with arguments
        self.s.set(task_path="example_task")
        task_callable = self.eval("task(task_path)")

        # Ensure the callable is valid
        self.assertTrue(callable(task_callable), "'task' did not return a callable object.")

        # Execute the task with additional arguments
        self.s.set(arg1="value1", arg2="value2")
        result = self.eval("task_callable(arg1=arg1, arg2=arg2)")

        # Assert the result is not None
        self.assertIsNotNone(result, "The task function returned None.")

        # Print the result for debugging
        print("Result:", result)
