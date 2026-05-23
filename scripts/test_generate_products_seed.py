"""generate_products_seed.py のユニットテスト."""

from __future__ import annotations

from pathlib import Path

import pytest
import yaml

import generate_products_seed as gen


def _write_yaml(path: Path, data: dict) -> None:
    path.write_text(yaml.safe_dump(data, sort_keys=False, allow_unicode=True), encoding="utf-8")


class TestGenerateFactionSet:
    """faction_set 商品が本表 + product_faction + product_card_pack の 3 INSERT に展開されることを固定する."""

    def test_emits_three_upsert_blocks(self, tmp_path: Path) -> None:
        src = tmp_path / "products.yaml"
        out = tmp_path / "out.sql"
        _write_yaml(
            src,
            {
                "products": [
                    {
                        "product_id": "faction_set_she",
                        "name": "SHE Pack",
                        "type": "faction_set",
                        "price": 980,
                        "description": "SHE 陣営のカードセット",
                        "is_active": True,
                        "faction": "SHE",
                        "card_pack_id": "faction_set_she",
                    },
                ]
            },
        )
        gen.generate(src, out)
        sql = out.read_text(encoding="utf-8")
        assert "Auto-generated" in sql
        assert "INSERT INTO shop.products" in sql
        assert "('faction_set_she', 'SHE Pack', 'faction_set', 980, 'SHE 陣営のカードセット', NULL, true)" in sql
        assert "INSERT INTO shop.product_faction" in sql
        assert "('faction_set_she', 'SHE')" in sql
        assert "INSERT INTO shop.product_card_pack" in sql
        assert "('faction_set_she', 'faction_set_she')" in sql


class TestIdempotency:
    """ON CONFLICT DO UPDATE で再実行が安全であることを固定する (card と同じ pattern)."""

    def test_products_upsert_clause(self, tmp_path: Path) -> None:
        src = tmp_path / "products.yaml"
        out = tmp_path / "out.sql"
        _write_yaml(
            src,
            {
                "products": [
                    {"product_id": "x", "name": "A", "type": "faction_set", "price": 1, "is_active": True, "faction": "SHE", "card_pack_id": "faction_set_she"},
                ]
            },
        )
        gen.generate(src, out)
        sql = out.read_text(encoding="utf-8")
        # 本表
        assert "ON CONFLICT (product_id) DO UPDATE SET" in sql
        assert "name        = EXCLUDED.name" in sql
        assert "price       = EXCLUDED.price" in sql
        # 副表
        assert "faction = EXCLUDED.faction" in sql
        assert "card_pack_id = EXCLUDED.card_pack_id" in sql


class TestGenerateCardPack:
    """card_pack 商品は本表 + product_card_pack のみ (faction 副表は出さない)."""

    def test_emits_two_upsert_blocks(self, tmp_path: Path) -> None:
        src = tmp_path / "products.yaml"
        out = tmp_path / "out.sql"
        _write_yaml(
            src,
            {
                "products": [
                    {
                        "product_id": "limited_2026_summer",
                        "name": "Summer 2026",
                        "type": "card_pack",
                        "price": 480,
                        "is_active": True,
                        "card_pack_id": "limited_2026_summer",
                    },
                ]
            },
        )
        gen.generate(src, out)
        sql = out.read_text(encoding="utf-8")
        assert "INSERT INTO shop.products" in sql
        assert "INSERT INTO shop.product_card_pack" in sql
        assert "shop.product_faction" not in sql


class TestSqlEscaping:
    """SQL リテラル埋め込みでシングルクオート文字列をエスケープする."""

    def test_apostrophe_in_description(self, tmp_path: Path) -> None:
        src = tmp_path / "products.yaml"
        out = tmp_path / "out.sql"
        _write_yaml(
            src,
            {
                "products": [
                    {
                        "product_id": "test_p",
                        "name": "Bob's Pack",
                        "type": "faction_set",
                        "price": 100,
                        "description": "with 'quote'",
                        "is_active": True,
                        "faction": "SHE",
                        "card_pack_id": "faction_set_she",
                    },
                ]
            },
        )
        gen.generate(src, out)
        sql = out.read_text(encoding="utf-8")
        assert "'Bob''s Pack'" in sql
        assert "'with ''quote'''" in sql


class TestValidation:
    """validate のエラー仕様."""

    def test_missing_required_key_raises(self):
        with pytest.raises(ValueError, match=r"missing required key 'name'"):
            gen.validate([{"product_id": "x", "type": "faction_set", "price": 0, "is_active": True}])

    def test_duplicate_product_id_raises(self):
        with pytest.raises(ValueError, match=r"duplicate product_id 'x'"):
            gen.validate([
                {"product_id": "x", "name": "A", "type": "faction_set", "price": 1, "is_active": True, "faction": "SHE", "card_pack_id": "faction_set_she"},
                {"product_id": "x", "name": "B", "type": "faction_set", "price": 1, "is_active": True, "faction": "Tenki", "card_pack_id": "faction_set_tenki"},
            ])

    def test_unsupported_type_raises(self):
        with pytest.raises(ValueError, match=r"unsupported type 'ghost'"):
            gen.validate([
                {"product_id": "x", "name": "A", "type": "ghost", "price": 1, "is_active": True},
            ])

    def test_faction_set_without_faction_raises(self):
        with pytest.raises(ValueError, match=r"requires 'faction'"):
            gen.validate([
                {"product_id": "x", "name": "A", "type": "faction_set", "price": 1, "is_active": True, "card_pack_id": "faction_set_she"},
            ])

    def test_faction_set_without_card_pack_id_raises(self):
        with pytest.raises(ValueError, match=r"requires 'card_pack_id'"):
            gen.validate([
                {"product_id": "x", "name": "A", "type": "faction_set", "price": 1, "is_active": True, "faction": "SHE"},
            ])


class TestNullableFields:
    """description / image_url 等の nullable フィールドは省略可能で NULL に展開する."""

    def test_missing_description_emits_null(self, tmp_path: Path) -> None:
        src = tmp_path / "products.yaml"
        out = tmp_path / "out.sql"
        _write_yaml(
            src,
            {
                "products": [
                    {
                        "product_id": "x",
                        "name": "A",
                        "type": "faction_set",
                        "price": 100,
                        "is_active": True,
                        "faction": "SHE",
                        "card_pack_id": "faction_set_she",
                    },
                ]
            },
        )
        gen.generate(src, out)
        sql = out.read_text(encoding="utf-8")
        # description, image_url とも未指定で NULL
        assert "NULL, NULL" in sql
