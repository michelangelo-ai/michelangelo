from unittest import TestCase
from michelangelo.lib.model_manager._private.constants import LLMModelType
from michelangelo.lib.model_manager._private.packager.template_renderer import TritonTemplateRenderer
from michelangelo.lib.model_manager._private.packager.llm_triton import generate_user_model_content


class UserModelPyTest(TestCase):
    def test_generate_user_model_content(self):
        gen = TritonTemplateRenderer()

        llm_model_type = LLMModelType.UNKNOWN

        model_def_script = "default/vllm"
        content = generate_user_model_content(gen, llm_model_type, model_def_script)
        self.assertEqual(content, gen.render("vllm/user_model.py.tmpl"))

        model_def_script = "default/python"
        content = generate_user_model_content(gen, llm_model_type, model_def_script)
        self.assertEqual(content, gen.render("python/user_model.py.tmpl"))

        model_def_script = "default/hf"
        content = generate_user_model_content(gen, llm_model_type, model_def_script)
        self.assertEqual(content, gen.render("hf/user_model.py.tmpl"))

        model_def_script = None
        content = generate_user_model_content(gen, llm_model_type, model_def_script)
        self.assertEqual(content, gen.render("vllm/user_model.py.tmpl"))

        model_def_script = "michelangelo/lib/model_manager/_private/packager/llm_triton/tests/fixtures/user_model.py.txt"
        content = generate_user_model_content(gen, llm_model_type, model_def_script)
        self.assertEqual(content, gen.render("hf/user_model.py.tmpl"))

        llm_model_type = LLMModelType.T5
        content = generate_user_model_content(gen, llm_model_type, None)
        self.assertEqual(content, gen.render("t5/user_model.py.tmpl"))
