"""generate_products_seed.py のユニットテスト."""

from __future__ import annotations

from pathlib import Path

import pytest
import yaml

import generate_products_seed as gen


def _write_yaml(path: Path, data: dict) -> None:
    path.write_text(yaml.safe_dump(data, sort_keys=False, allow_unicode=True), encoding="utf-8")


class TestFactionSet商品のSQL生成:
    def test_本表とproduct_factionとproduct_card_packの3ブロックに展開する(self, tmp_path: Path) -> None:
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


class Test再実行の冪等性:
    def test_ON_CONFLICT_DO_UPDATEで再実行を安全にする(self, tmp_path: Path) -> None:
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


class TestCardPack商品のSQL生成:
    def test_本表とproduct_card_packのみ生成しproduct_factionは出さない(self, tmp_path: Path) -> None:
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


class TestSQLリテラルのエスケープ:
    def test_説明文中のシングルクオートをエスケープする(self, tmp_path: Path) -> None:
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


class Test定義の検証:
    @pytest.mark.parametrize(
        ("products", "match"),
        [
            pytest.param(
                [{"product_id": "x", "type": "faction_set", "price": 0, "is_active": True}],
                r"missing required key 'name'",
                id="必須キー name が無いとき、エラーになる",
            ),
            pytest.param(
                [
                    {"product_id": "x", "name": "A", "type": "faction_set", "price": 1, "is_active": True, "faction": "SHE", "card_pack_id": "faction_set_she"},
                    {"product_id": "x", "name": "B", "type": "faction_set", "price": 1, "is_active": True, "faction": "Tenki", "card_pack_id": "faction_set_tenki"},
                ],
                r"duplicate product_id 'x'",
                id="product_id が重複するとき、エラーになる",
            ),
            pytest.param(
                [{"product_id": "x", "name": "A", "type": "ghost", "price": 1, "is_active": True}],
                r"unsupported type 'ghost'",
                id="未対応の type のとき、エラーになる",
            ),
            pytest.param(
                [{"product_id": "x", "name": "A", "type": "faction_set", "price": 1, "is_active": True, "card_pack_id": "faction_set_she"}],
                r"requires 'faction'",
                id="faction_set に faction が無いとき、エラーになる",
            ),
            pytest.param(
                [{"product_id": "x", "name": "A", "type": "faction_set", "price": 1, "is_active": True, "faction": "SHE"}],
                r"requires 'card_pack_id'",
                id="faction_set に card_pack_id が無いとき、エラーになる",
            ),
        ],
    )
    def test_不正な定義を拒否する(self, products, match):
        with pytest.raises(ValueError, match=match):
            gen.validate(products)


class Test省略可能フィールドのNULL展開:
    def test_description未指定はNULLに展開する(self, tmp_path: Path) -> None:
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
        # description・image_url とも未指定なので 2 つ連続で NULL になる
        assert "NULL, NULL" in sql
