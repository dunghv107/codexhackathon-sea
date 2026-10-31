from unittest.mock import AsyncMock

import httpx
import pytest
from fastapi import HTTPException

from app.api.app import create_app
from app.api.kbs_dependencies import order_service, shop_id
from app.services.voice_service import VoiceResult

pytestmark = pytest.mark.component


@pytest.mark.parametrize("failure", ["foreign", "provider", "disabled", "none"])
async def test_voice_preview_authorizes_before_provider_and_sanitizes_errors(failure: str) -> None:
    app = create_app()
    orders = AsyncMock()
    if failure == "foreign":
        orders.detail.side_effect = HTTPException(404, "Not found")
    app.dependency_overrides[shop_id] = lambda: "shop"
    app.dependency_overrides[order_service] = lambda: orders
    voice = AsyncMock()
    voice.preview.return_value = VoiceResult(transcript="Có", intent="confirmed")
    if failure == "provider":
        voice.preview.side_effect = httpx.ReadTimeout("private provider key")
    app.state.voice = None if failure == "disabled" else voice
    async with httpx.AsyncClient(transport=httpx.ASGITransport(app), base_url="http://test") as client:
        result = await client.post(
            "/api/v1/orders/order/voice-preview", json={"audio": "YQ==", "mime_type": "audio/wav"}
        )
    assert result.status_code == {"foreign": 404, "provider": 503, "disabled": 503, "none": 200}[failure]
    assert "private" not in result.text
    orders.detail.assert_awaited_once_with("order", "shop")
    if failure in {"foreign", "disabled"}:
        voice.preview.assert_not_awaited()
