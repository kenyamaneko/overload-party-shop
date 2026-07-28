# IAP シークレットの投入手順

shop は `IAP_MODE=production` の環境で、起動時に Secret Manager から Apple / Google の課金検証用シークレットを読む。シークレットの入れ物と読み取り権限は Terraform (overload-party-infra の `env/modules/app/shop/shop-secrets`) が作るが、**値は人が投入する**。値が 1 つでも欠けていると shop は起動に失敗する。

本書は環境を新しく立ち上げるとき、および Apple の鍵をローテーションするときに実施する。

## 投入するシークレット

| シークレット ID | 値 | 入手元 |
|---|---|---|
| `shop-apple-key-id` | In-App Purchase 用 API キーの Key ID | App Store Connect > ユーザーとアクセス > 統合 > App Store Connect API > In-App Purchase |
| `shop-apple-issuer-id` | 同ページの Issuer ID | 同上 |
| `shop-apple-bundle-id` | アプリの Bundle ID | App Store Connect のアプリ情報。クライアントの `capacitor.config.ts` の `appId` と一致する |
| `shop-apple-private-key` | 上記 API キーの秘密鍵 (`.p8` ファイルの中身) | キー作成時に一度だけダウンロードできる。PKCS#8 PEM のまま投入する |
| `shop-google-package-name` | Android アプリのパッケージ名 | Google Play Console のアプリ。クライアントの `appId` と一致する |

秘密鍵は起動時に PKCS#8 の EC 鍵としてパースされる。改行を含む PEM をそのまま入れる必要があり、仮の文字列では起動しない。

## 手順

環境ごとに実施する。`<env>` は `dev` / `stg` / `prod` のいずれか。

1. 値を投入する。ファイルから読ませる形にして、シェル履歴に秘密鍵が残らないようにする

   ```bash
   PROJECT=overload-party-<env>

   printf '%s' "<KEY_ID>"       | gcloud secrets versions add shop-apple-key-id        --project "$PROJECT" --data-file=-
   printf '%s' "<ISSUER_ID>"    | gcloud secrets versions add shop-apple-issuer-id     --project "$PROJECT" --data-file=-
   printf '%s' "<BUNDLE_ID>"    | gcloud secrets versions add shop-apple-bundle-id     --project "$PROJECT" --data-file=-
   printf '%s' "<PACKAGE_NAME>" | gcloud secrets versions add shop-google-package-name --project "$PROJECT" --data-file=-
   gcloud secrets versions add shop-apple-private-key --project "$PROJECT" --data-file=AuthKey_<KEY_ID>.p8
   ```

2. 5 つとも版が付いたことを確認する

   ```bash
   for s in shop-apple-key-id shop-apple-issuer-id shop-apple-bundle-id \
            shop-apple-private-key shop-google-package-name; do
     echo "$s: $(gcloud secrets versions list "$s" --project "$PROJECT" --format='value(name)' | wc -l)"
   done
   ```

3. shop の Cloud Run サービスに新しいリビジョンを出す。shop は起動時に一度だけ読むため、値を入れただけでは反映されない

   ```bash
   gcloud run services update shop --project "$PROJECT" --region asia-northeast1 --no-traffic --tag=reload
   ```

   通常は `Deploy` workflow の再実行でよい。

4. 起動を確認する

   ```bash
   gcloud run services describe shop --project "$PROJECT" --region asia-northeast1 \
     --format='value(status.conditions[0].status,status.latestReadyRevisionName)'
   ```

   起動に失敗している場合はログに落ちた理由が出る。

   ```bash
   gcloud logging read 'resource.labels.service_name="shop" AND severity>=ERROR' \
     --project "$PROJECT" --limit 5 --freshness=10m --format='value(textPayload)'
   ```

## 環境ごとの Apple 環境

infra の `shop_apple_environment` が dev / stg で `Sandbox`、prod で `Production` になっている。Sandbox の環境には Sandbox のテスターで購入したレシートしか通らない。鍵そのものは App Store Connect の同じ API キーを使う。

## 鍵をローテーションするとき

App Store Connect で新しい API キーを作り、手順 1 の `shop-apple-key-id` と `shop-apple-private-key` に新しい版を追加してから手順 3 を実施する。shop は常に最新版を読むため、リビジョンを出し替えた時点で切り替わる。旧キーは切り替えを確認してから App Store Connect 側で失効させる。
