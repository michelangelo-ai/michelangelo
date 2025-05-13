from uber.ai.michelangelo.sdk.api_client.v2beta1 import APIClient

if not APIClient._context.header_provider._caller:
    APIClient.set_caller("ml-code")
