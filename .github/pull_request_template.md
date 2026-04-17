## 変更内容

<!-- 何をなぜ変えたかを書く -->

## 種別

- [ ] feature (develop にマージ)
- [ ] release (main にマージ)
- [ ] hotfix (main にマージ)
- [ ] back-merge (release → develop)
- [ ] back-merge (hotfix → develop)

## hotfix / release の場合

- [ ] main にマージ予定 / 済み
- [ ] develop への back-merge PR を作成予定 / 済み
- [ ] (release 中の場合) release への back-merge PR を作成予定 / 済み

## 確認事項

- [ ] テストを追加・更新した
- [ ] ドキュメントを更新した(該当する場合)
- [ ] CLAUDE.md の禁止事項に触れていない
  - [ ] 生成済み型コードを手で書き換えていない
  - [ ] 手動で git tag を打っていない
- [ ] stg 環境で検証した(release の場合)