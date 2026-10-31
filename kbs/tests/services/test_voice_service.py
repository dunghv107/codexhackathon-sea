import base64
import json

import httpx
import pytest
from pydantic import ValidationError

from app.services.voice_service import VoiceInput, VoiceService

pytestmark = pytest.mark.component


@pytest.mark.parametrize("intent", ["confirmed", "declined", "unclear"])
async def test_voice_preview_transcribes_then_classifies_without_order_data(intent: str) -> None:
    calls = []

    def handle(request: httpx.Request) -> httpx.Response:
        calls.append(request.url.path)
        if request.url.path.endswith("transcriptions"):
            body = request.read()
            assert b'name="model"\r\n\r\nasr' in body
            assert b"voice-content" in body
            return httpx.Response(200, json={"text": "Mai giao sau năm giờ nhé."})
        payload = json.loads(request.content)
        assert payload["store"] is False
        assert json.loads(payload["input"]) == {"transcript": "Mai giao sau năm giờ nhé."}
        assert payload["text"]["format"]["strict"] is True
        return httpx.Response(
            200,
            json={
                "status": "completed",
                "output": [
                    {"type": "message", "content": [{"type": "output_text", "text": json.dumps({"intent": intent})}]}
                ],
            },
        )

    async with httpx.AsyncClient(transport=httpx.MockTransport(handle)) as client:
        result = await VoiceService(client, "https://provider.invalid/v1", "asr", "classifier").preview(
            VoiceInput(audio=base64.b64encode(b"voice-content").decode(), mime_type="audio/webm")
        )
    assert result.intent == intent
    assert result.transcript == "Mai giao sau năm giờ nhé."
    assert calls == ["/v1/audio/transcriptions", "/v1/responses"]


async def test_silence_is_unclear_without_classification() -> None:
    calls = []

    def handle(request: httpx.Request) -> httpx.Response:
        calls.append(request.url.path)
        return httpx.Response(200, json={"text": " "})

    async with httpx.AsyncClient(transport=httpx.MockTransport(handle)) as client:
        result = await VoiceService(client, "https://provider.invalid", "asr", "classifier").preview(
            VoiceInput(audio="YQ==", mime_type="audio/wav")
        )
    assert result.intent == "unclear"
    assert len(calls) == 1


@pytest.mark.parametrize(
    "response",
    [
        {"status": "incomplete"},
        {"status": "completed", "output": []},
        {
            "status": "completed",
            "output": [{"type": "message", "content": [{"type": "output_text", "text": '{"intent":"release"}'}]}],
        },
    ],
)
async def test_invalid_classification_never_becomes_a_confirmation(response: dict[str, object]) -> None:
    async with httpx.AsyncClient(
        transport=httpx.MockTransport(
            lambda request: httpx.Response(
                200, json={"text": "Có thể."} if request.url.path.endswith("transcriptions") else response
            )
        )
    ) as client:
        with pytest.raises(ValueError, match=r".+"):
            await VoiceService(client, "https://provider.invalid", "asr", "classifier").preview(
                VoiceInput(audio="YQ==", mime_type="audio/wav")
            )


async def test_bad_audio_does_not_call_provider() -> None:
    async with httpx.AsyncClient(transport=httpx.MockTransport(lambda _: pytest.fail("Provider called"))) as client:
        with pytest.raises(ValueError, match=r".+"):
            await VoiceService(client, "https://provider.invalid", "asr", "classifier").preview(
                VoiceInput(audio="!!!!", mime_type="audio/wav")
            )
    with pytest.raises(ValidationError):
        VoiceInput.model_validate({"audio": "YQ==", "mime_type": "text/plain"})
