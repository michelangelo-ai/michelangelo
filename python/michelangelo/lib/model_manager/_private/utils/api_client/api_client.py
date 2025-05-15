from michelangelo.api.v2 import APIClient

if not APIClient._context.header_provider._caller:
    APIClient.set_caller("ml-code")
