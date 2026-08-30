<img src="docs/logo-horizontal.png" alt="HAMi-WebUI Logo (Light)" width="50%">

[English](README.md) | [简体中文](README_ZH.md) | 日本語

[HAMi](https://github.com/Project-HAMi/HAMi) が管理する異種アクセラレータ向けの、オープンソースかつシングルクラスタ対応の可観測性 UI

[![License](https://img.shields.io/github/license/Project-HAMi/HAMi-WebUI)](LICENSE)

HAMi-WebUI は、Kubernetes ノード上のアクセラレータのインベントリ、割り当て状態、および利用可能なベンダーテレメトリを可視化します。主な機能は次のとおりです。

- **クラスタ概要：** 検出されたアクセラレータの容量、割り当て、取得可能な使用率指標を確認します。
- **ノードインベントリ：** アクセラレータノード、デバイス、およびリソース状態を確認します。
- **アクセラレータ可視性：** デバイスごとの割り当てとベンダーテレメトリを確認します。
- **ワークロード可視性：** 現在観測されている HAMi ワークロード（Pod/コンテナ）へのデバイス割り当てと、対象デバイスで利用可能な監視データを確認します。

現在の連携対象は、NVIDIA GPU、Huawei Ascend 910B/310P、Hygon DCU、Cambricon MLU、および MetaX GPU/sGPU です。表示できるメトリクスは、デバイス種別ごとの HAMi 連携と exporter によって異なります。

## スコープとセキュリティ境界

HAMi-WebUI は読み取り専用です。リソースのスケジューリング、ワークロードの作成や変更、ノードの変更、組み込み認証やユーザー RBAC、マルチクラスタ集約は行いません。クラスタごとに 1 インスタンスをデプロイし、信頼されたネットワーク内だけで利用する場合を除き、認証付き Ingress またはプロキシで保護してください。

## はじめに

- [インストールガイド](docs/installation/helm/index.md)
- [URL プレフィックスとプラットフォームへの埋め込み](docs/installation/embedding.md)

## コントリビューション

HAMi-WebUI への貢献に興味がある方は：

- まず[コントリビューションガイド](CONTRIBUTING.md)をお読みください。
- [開発者ガイド](docs/contribute/developer-guide.md)に従ってローカル環境をセットアップしてください。
- [Good first issue](https://github.com/Project-HAMi/HAMi-WebUI/issues?q=is%3Aopen+is%3Aissue+label%3A%22good+first+issue%22) を確認してみてください。

## コミュニティ

HAMi-WebUI は HAMi コミュニティの共通チャンネルを利用しています。最新のコミュニティミーティング、Discord、Slack、およびメーリングリストについては、[HAMi のコミュニティ案内](https://github.com/Project-HAMi/HAMi/blob/master/README_ja.md#コミュニティ) を参照してください。

## ライセンス

HAMi-WebUI は [Apache-2.0](LICENSE) ライセンスの下で配布されています。詳細については [LICENSE](LICENSE) をご覧ください。
