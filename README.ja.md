<div align="center">
  <img src="apps/desktop/Resources/OpenSurgeAppIcon.png" width="96" height="96" alt="OpenSurge for Mac のアプリアイコン">
  <h1>OpenSurge for Mac</h1>
  <p><strong>Mac を、デバイスごとに経路を振り分けられる Surge 風の家庭全体向け透過ゲートウェイに変えます。同一 LAN のバイパスルーターモードで選んだデバイスだけを接続したり、DHCP/DNS を自動で引き継いだり、実験的な下流 IPv6 の引き継ぎを有効にしたりできます。</strong></p>
  <p>
    <a href="https://github.com/YTwsy/OpenSurge-for-Mac/releases"><img alt="最新リリース" src="https://img.shields.io/github/v/release/YTwsy/OpenSurge-for-Mac?style=flat-square"></a>
    <a href="https://opensurge.pages.dev/"><img alt="OpenSurge の Web サイト" src="https://img.shields.io/badge/website-opensurge.pages.dev-2a7b62?style=flat-square"></a>
    <img alt="macOS 13+" src="https://img.shields.io/badge/macOS-13%2B-000000?style=flat-square&amp;logo=apple">
    <img alt="Apple Silicon 版と Intel 版のパッケージ" src="https://img.shields.io/badge/Apple%20Silicon%20%7C%20Intel-packages-6f42c1?style=flat-square&amp;logo=apple">
    <a href="LICENSE"><img alt="GPL-3.0-only" src="https://img.shields.io/badge/license-GPL--3.0--only-2ea44f?style=flat-square"></a>
  </p>
  <p>
    <a href="README.md">简体中文</a> · <a href="README.en.md">English</a> · <strong>日本語</strong>
  </p>
  <p>
    <a href="https://opensurge.pages.dev/">Web サイト</a> ·
    <a href="https://github.com/YTwsy/OpenSurge-for-Mac/releases">ダウンロード</a> ·
    <a href="https://opensurge.pages.dev/docs/">ドキュメント</a> ·
    <a href="docs/user/app-user-guide.md">アプリガイド</a> ·
    <a href="#capabilities">機能</a> ·
    <a href="#per-device-policies">デバイスごとのポリシー</a> ·
    <a href="#web-gui-and-menu-bar-app">Web GUI</a> ·
    <a href="#an-ai-agent-friendly-engineering-workspace">Agent ワークスペース</a>
  </p>
  <table width="100%">
    <tr>
      <td width="66%" valign="top">
        <img src="docs/images/readme/en/dashboard.png" width="100%" alt="OpenSurge の家庭全体ゲートウェイのダッシュボード">
      </td>
      <td width="34%" valign="top">
        <img src="docs/images/readme/en/policies.png" width="100%" alt="OpenSurge のポリシーとプロキシのヘルス状態の画面">
        <br>
        <img src="docs/images/readme/en/devices.png" width="100%" alt="OpenSurge のデバイスごとのポリシーの画面">
      </td>
    </tr>
  </table>
  <p><sub>スクリーンショットには、隔離された架空のデモデータを使用しています。</sub></p>
</div>

OpenSurge for Mac は、オープンソースで Surge 風の macOS 用ゲートウェイ兼コントロールプレーンです。ほとんどのユーザーは、同一 LAN のバイパスルーターモードから始められます。メインルーターの DHCP は有効のままにし、選んだデバイスに固定の IPv4 アドレスを割り当て、そのゲートウェイと DNS を Mac に向けます。既存の LAN 全体で自動的に接続させたい場合は LAN の DHCP 引き継ぎを、別の AP、SSID、VLAN を使う場合は隔離された下流 LAN を選びます。3 つのモードはいずれも、実験的な下流 IPv6 の引き継ぎを任意で有効にできます。

どのモードでも、登録したデバイスごとに独立した出口ポリシーを設定できます。たとえば、スマートフォンとローカルの Mac にはルールベースのルーティングを使い、ゲーム機は米国リージョンのノード経由に、テレビはストリーミング用のノード経由にできます。LAN の DHCP 引き継ぎモードと隔離された下流 LAN モードでは、スマートフォン、テレビ、PS5、VR ヘッドセットが Mac から自動的に DHCP/DNS を取得するため、デバイスごとにゲートウェイや DNS を変更する必要はありません。

> 機能の概要、セットアップ手順、実践的なガイドについては、[OpenSurge の Web サイト](https://opensurge.pages.dev/) をご覧ください。
>
> [機能](https://opensurge.pages.dev/features/) · [ガイド](https://opensurge.pages.dev/guides/) · [ドキュメント](https://opensurge.pages.dev/docs/)

<details>
  <summary><strong>10 枚の画像で見る OpenSurge</strong></summary>

  <p>横にスクロールすると、ギャラリー全体を見られます。</p>

  <pre><img src="docs/images/readme/gallery-en/01-home-gateway.png" width="480" alt="Mac を家庭全体のネットワークゲートウェイに"><img src="docs/images/readme/gallery-en/02-per-device-routing.png" width="480" alt="デバイスごとに独立した経路"><img src="docs/images/readme/gallery-en/03-network-topologies.png" width="480" alt="1 台のデバイスから始められる 3 つのネットワーク構成"><img src="docs/images/readme/gallery-en/04-prepare-policies.png" width="480" alt="ゲートウェイを起動する前にポリシーを準備してプレビュー"><img src="docs/images/readme/gallery-en/05-network-overview.png" width="480" alt="ゲートウェイの状態、ライブトラフィック、アクティブなデバイス"><img src="docs/images/readme/gallery-en/06-independent-mac-routing.png" width="480" alt="ローカルの Mac 用の独立したルーティング設定"><img src="docs/images/readme/gallery-en/07-reusable-rules.png" width="480" alt="再利用できるルールセットとルーティングテンプレート"><img src="docs/images/readme/gallery-en/08-diagnostics-recovery.png" width="480" alt="診断とガイド付きのネットワーク復旧"><img src="docs/images/readme/gallery-en/09-optional-connectivity.png" width="480" alt="任意の Tailscale アウトバウンドと実験的な下流 IPv6"><img src="docs/images/readme/gallery-en/10-get-opensurge.png" width="480" alt="OpenSurge for Mac を入手して Web サイトにアクセス"></pre>
</details>

| モード | 適した用途 | 既存のネットワークへの影響 |
| --- | --- | --- |
| **同一 LAN のバイパスルーターモード（一般的・最初のステップとして推奨）** | 選んだスマートフォン、テレビ、ゲーム機などのデバイスから始める場合 | メインルーターの DHCP は有効のままです。選んだデバイスは固定の IPv4 アドレスを使い、ゲートウェイと DNS を手動で Mac に向けます |
| **LAN の DHCP 引き継ぎ（上級者向け・自動接続）** | 同じ LAN 上のデバイスを自動的に OpenSurge に接続する場合 | ガイド付きの手順に従ってメインルーターの DHCP を無効にし、停止時に元に戻します |
| **隔離された下流 LAN** | 別の AP、SSID、VLAN を使う場合 | 既存の LAN の DHCP は変更されません。Mac が隔離された下流ネットワークに DHCP/DNS とゲートウェイを提供します |

- mihomo のプロファイルやサブスクリプションのインポートは任意です。デフォルトで折りたたまれている **Global Profile Overlay** パネルでは、プロキシ、プロバイダー、ポリシーグループ、DNS 設定、ルールを独立して追加できます。ゲートウェイが停止していてインポート元がなくても、Policies ページで最終的に合成された設定のプレビュー、ノードの選択、レイテンシのテストができ、その設定のまま Web GUI から直接起動することもできます。サブスクリプションの更新や下書きの保存によって、実行中のゲートウェイが自動的に変わることはありません。
- Web GUI とメニューバーアプリで、どのデバイスがアクティブか、どれだけのトラフィックを流しているか、どの出口チェーンを使っているかを確認できます。
- どちらの UI からでも、蓋を閉じた状態で Mac を一時的に動作させ続けられます。このスイッチはデフォルトでオフで、現在の OpenSurge の実行中にのみ適用されます。

内部では、dnsmasq が DHCP/DNS を提供し、mihomo がプロキシエンジンとして動作します。IPv4 はネイティブのゲートウェイ経路として macOS の pf とフォワーディングを使用します。実験的な下流 IPv6 では、dnsmasq の RA/SLAAC/RDNSS、macOS の BPF パケットブローカー、OpenSurge でパッチを当てた mihomo ビルドのユーザー空間データプレーンを使用します。

このリポジトリは、[AI エージェントにやさしいエンジニアリングワークスペース](#an-ai-agent-friendly-engineering-workspace) としても設計されています。プロジェクトの知識はコードと並べてバージョン管理され、リスクのあるネットワークの挙動には実行可能な検証ゲートがあり、仮想ラボと実機での検証結果は次のエンジニアリングのループに反映されます。

<a id="capabilities"></a>
## 機能

**使いやすいアプリ体験**

- macOS のメニューバーアプリで、状態の確認、復旧に関する警告の受信、ローカルの Web GUI を開く操作ができます。
- ゲートウェイの状態とは別に、アイドル時や蓋を閉じたときのスリープを一時的に防止できます。この設定は保存されず、OpenSurge を終了するか Mac を再起動すると解除されます。
- ソースのインポート、ネットワークの設定、デバイスごとのルーティング、プロキシのヘルス状態と接続性の確認、診断の確認を 1 つの画面で行えます。
- 同一 LAN での DHCP 引き継ぎの起動、クライアントの検証、停止、ネットワークの復元を、復旧用の状態遷移に沿って進められます。

まずは [OpenSurge for Mac アプリユーザーガイド](docs/user/app-user-guide.md) から始めてください。ローカルネットワーク、TUN、デバイスの設定に関するよくある質問については、[FAQ](docs/user/faq.md) を参照してください。

**ゲートウェイとプロキシ**

- DHCP/DNS、mihomo、pf NAT、IPv4 フォワーディングを、ロールバック付きで起動・停止します。
- mihomo の `mixed-port` を通じて明示的なプロキシを提供します。
- macOS 上で mihomo の TUN を通じて透過プロキシを提供します。
- 明示した MagicDNS のサフィックス、Tailnet のピア、承認済みのサブネットルート、または任意のデバイスごとの Exit Node 向けに、Tailscale または Headscale をオンデマンドの mihomo アウトバウンドとして使用できます。
- 実験的な下流 IPv6 モードでは、隔離された LAN と LAN 全体の DHCP 引き継ぎは自動接続のために dnsmasq の RA/SLAAC/RDNSS を使用し、バイパスルーターモードは手動の ULA を使用します。IPv6 のトラフィックは macOS のシステム TUN には入りません。BPF パケットブローカーが OpenSurge でパッチを当てた mihomo の gVisor データプレーンに送り、TCP、UDP、UDP/443 で運ばれる QUIC を扱いつつ、MAC アドレスに基づくデバイスの識別を維持します。
- TUN または明示的なプロキシに入るローカルの Mac の新しい接続について、下流のデバイスを変えずに **Rule / Global / Direct** を切り替えられます。OpenSurge はデフォルトでは macOS のシステムプロキシの設定を変更しません。SafeDNS、DNS Proxy、その他の Network Extension との競合に備えて、TUN 専用の HTTP/HTTPS 連携オプションを明示的に用意しています。
- DHCP 引き継ぎモードでは MAC アドレスに基づく固定の IPv4 リースを生成し、同一 LAN の手動ゲートウェイモードではメインルーターで固定した IPv4 を使用します。どちらの構成でも、独立した出口ポリシーを設定できます。

**可観測性**

- アクティブなセッションのトラフィックを、DHCP のデバイス、または同一 LAN で登録済みかつ現在検出されているデバイスに紐付け、デバイスごとの接続数、ライブのアップロード/ダウンロード速度、累計バイト数、主に使われている mihomo の出口チェーンを表示します。
- ゲートウェイの実行中でも停止中でも、プロキシノードの到達性とレイテンシをテストし、最終的に合成されたポリシーの画面から Selector を切り替えられます。
- 適用済みの mihomo の mixed-port と現在のローカルの Mac のモードを通じて、決まった実サービスの一覧に対してプローブを行い、3 回の中央値のレイテンシ、マッチしたルール、実際の出口チェーンを表示します。
- ポリシーグループの確認と切り替え、インポートしたプロキシ/ルールプロバイダーの状態の確認、現在の接続の表示ができます。
- テキスト/JSON 形式の status、doctor、logs、snapshot を出力できます。診断や UI 向けに、一部が失敗した場合の JSON スナップショットも含まれます。

**安全性と検証**

- 設定の検証、TUN 専用の透過プロキシ、ロールバック、明示的な復旧の取り決め。
- 通常の LAN に触れる前に、リスクのあるネットワークの挙動を隔離された仮想 LAN で検証します。

<a id="per-device-policies"></a>
## デバイスごとのポリシー

1 つの mihomo プロセスで、登録した LAN デバイスごとに独立したポリシーを適用できます。DHCP 引き継ぎモードでは、各デバイスに MAC アドレスに基づく固定の IPv4 リースを割り当てます。同一 LAN の手動ゲートウェイモードでは、代わりにメインルーターで固定した IPv4 を使用し、現在のトラフィックと ARP の近隣情報の観測によって登録を補助できます。どちらの構成でも、デバイスごとの mihomo の selector グループと `SRC-IP-CIDR` ルールが出力されます。インストールしたアプリではデバイスごとのポリシーがデフォルトで有効になっており、Web GUI でも有効のまま維持されます。JSON のポリシーファイルでは、各デバイスについて、ゲートウェイのルールに従うか、グローバルルールより前に専用のデバイス selector を使うかを選べます。また、`REJECT` のようなデバイス固有の直接のアクションや、ドメイン/IP/プロトコル/ポート/ルールプロバイダーのオーバーレイにも対応しています。専用モードでも、ローカル/プライベートの宛先は直接接続のままです。ローカルの Mac の Rule / Global / Direct の切り替えによって、これらの下流のルールは変わりません。[ローカルの Mac のルーティングモード](docs/user/local-mac-routing.md) を参照してください。

Web GUI のルールライブラリでは、ルールセット、出口を含まないルーティングテンプレート、マッチ後に選ばれるデバイスごとの出口を、別々の概念として管理します。確認可能なコミュニティ製の Claude Code の例が含まれていますが、デフォルトではどのデバイスにも適用されません。それ以外のポリシーの内容はすべて運用者が用意します。空の初期ファイルもそのまま有効です。JSON のモデル、優先順位、CLI コマンド、検証の範囲については、[デバイスごとのポリシーオーバーレイ](docs/user/device-policy.md) を参照してください。

### Tailscale アウトバウンド

**Proxy and rule sources** ページでは、OpenSurge が所有する Tailscale アウトバウンドを 1 つ管理できます。Auth Key は書き込み専用で、別の `0600` ファイルに保存されます。永続的な `state-dir` によって、再読み込みや一時的な無効化/有効化を繰り返しても、同じローカルの Tailnet ID が維持されます。このローカルの ID を破棄する操作は別に用意されており、Tailscale とゲートウェイが停止している間のみ実行できます。また、この操作で Tailscale や Headscale の管理コンソールからデバイスが削除されることはありません。

折りたたみ可能なインラインのセットアップパネルは、ローカルの Tailscale アプリを検出専用モードで読み取り、現在の Tailnet の MagicDNS のサフィックス、正確なピアのアドレス、オンライン状態、承認済みのプライベートルート、利用可能な Exit Node を、確認が必要な候補として表示します。自動的に保存したり、アクセス範囲を広げたりすることはありません。検出に成功すると、認証情報を含まない制限付きのキャッシュが保存されるため、ローカルのアプリが切断された後も設定を続けられます。パネルにはスナップショットをライブの状態として見せるのではなく、情報の取得元と確認/キャッシュの時刻が表示されます。初回の登録には引き続き別の Auth Key が必要です。パネルは公式の Keys ページへのリンクを示し、1 回限りで Ephemeral ではないキーを推奨します。ローカルのアプリと OpenSurge が管理するノードは、ログイン ID や状態を共有しません。検出が利用できない場合は、高度な手動設定も引き続き利用できます。

Tailnet 専用のアクセスは宛先単位で限定され、失敗時は遮断されます（fail closed）。一般的なデバイスの出口になることはありません。デバイスごとの出口の selector に表示されるのは、Exit Node を明示したアウトバウンドだけです。リモートのサブネットルートは明示的に確認する必要があり、OpenSurge の LAN と重なる場合は拒否されます。mihomo は、アウトバウンドへの最初のリクエスト時に Tailscale ノードを遅延起動します。OpenSurge はゲートウェイの起動、再読み込み、mihomo の復旧の後にベストエフォートのウォームアップを送信しますが、アプリケーションの最初のリクエストでは再試行が必要になる場合があります。Tailnet 専用の状態は、公開の `generate_204` URL に対してテストするのではなく、オンデマンドのままです。

この連携はアウトバウンド専用です。OpenSurge は管理対象のノードを通じて、ローカルの LAN をアドバタイズしたり、サブネットルーターとして動作したり、インバウンドのサービスを公開したりはしません。

<a id="web-gui-and-menu-bar-app"></a>
## Web GUI とメニューバーアプリ

パッケージから OpenSurge をインストールした場合は、[OpenSurge for Mac アプリユーザーガイド](docs/user/app-user-guide.md) から始めてください。

v0.3.0 以降、PKG では、共通の React のメインウィンドウとトレイパネルを備えた、Wails + システム WebView のデスクトップアプリを使用しています。独立したループバックの Control API とブラウザ UI も引き続きサポートされています。v0.2.4 の Swift ホストは過去版の保守のために残されています。[インストーラーの受け入れ範囲](docs/agent-wiki/sources/validation/evidence-map.md#桌面与安装) を参照してください。開発用ビルドの場合:

```sh
make web-install
make control-build
./bin/opensurge-control --config examples/config.example.yaml
make desktop-production-build
```

コントロールサービスは `127.0.0.1` でのみ待ち受け、1 回限りの Web GUI の起動用リンクを出力します。メニューバーアプリは状態と復旧に関する警告を表示し、既存のデスクトップのメインウィンドウを開きます。独立した一時的な蓋を閉じた状態での動作のスイッチを除き、ゲートウェイの起動/停止やポリシーの選択の操作は意図的に用意していません。デスクトップアプリだけを終了する操作と、OpenSurge を終了する操作は分かれています。後者はゲートウェイのデータプレーンが停止した後にのみ利用でき、デスクトップアプリとユーザーレベルの Control Service を終了します。launchd が管理する root の Helper は読み込まれたままアイドル状態になるため、OpenSurge を再度開くときに管理者の承認を改めて求められることはありません。メニューバーには、別途 Uninstall OpenSurge の操作もあります。ゲートウェイを停止した後、macOS の管理者の承認によって、アプリ、Control Service、root の Helper を削除できます。その際、再インストール用に設定データを残すか、すべてを削除するかを選べます。Web GUI には、適用済みの設定と現在のローカルの Mac のモードを対象としたネイティブの接続性ページがあり、ブラウザ内で別途確認するための Net.Coffee へのリンクもあります。どちらの結果も、下流のゲートウェイのルールや、デバイスの DHCP/DNS/TUN の経路を証明するものとしては示されません。ローカルでのビルドについては [開発](docs/agent-wiki/sources/development.md) を、認証と権限については [Control API](docs/agent-wiki/sources/control-api.md) を、インストールとアンインストールについては [配布](docs/agent-wiki/sources/distribution.md) を参照してください。

`make gui-installer` は、実際の mihomo と dnsmasq のバイナリを必須として macOS のパッケージをビルドします。Developer ID による署名と公証は、アーキテクチャのメモに記載された環境変数で任意に有効にできます。安定版の GitHub リリースには、明示的に名前を付けた `arm64-unsigned.pkg` と `x86_64-unsigned.pkg` のビルドが含まれます。安定版の Release が、署名済み、公証済み、または Gatekeeper に対応済みと説明されることはありません。

### 未署名の安定版 GitHub リリースをインストールする

安定版のリリースでは、Apple Silicon と Intel の両方の Mac 向けにパッケージを提供しています。対応する GitHub Release から、Apple Silicon の場合は `arm64-unsigned.pkg`、Intel の場合は `x86_64-unsigned.pkg` と、`SHA256SUMS` をダウンロードします。ダウンロードしたファイルは `shasum -a 256 -c SHA256SUMS` で検証でき、選んだパッケージの GitHub のビルド来歴は次のコマンドで検証できます。

```sh
gh attestation verify OpenSurge-for-Mac-*-arm64-unsigned.pkg \
  -R YTwsy/OpenSurge-for-Mac
gh attestation verify OpenSurge-for-Mac-*-x86_64-unsigned.pkg \
  -R YTwsy/OpenSurge-for-Mac
```

パッケージをダブルクリックします。Gatekeeper にブロックされた場合は、**システム設定 → プライバシーとセキュリティ** を開いて **このまま開く** を選び、認証してから同じパッケージをもう一度開きます。Gatekeeper を全体的に無効にしたり、quarantine 属性を再帰的に削除したりしないでください。管理者アカウントでインストーラーを完了させ、`/Applications` から **OpenSurge** を開きます。インストールによってローカルの helper と Control Service が起動しますが、コントロールプレーンから明示的に起動するまで、ゲートウェイは停止したままです。

同一 LAN の DHCP の復旧が完了していない間は、パッケージのアップグレードは実行されません。ペイロードのファイルを置き換える前に、preinstall はメニューバーアプリを停止して Control Service を起動できないようにし、ユーザーの Control Service をアンロードし、新しいパッケージに埋め込まれた現在のバージョンの復旧 CLI をインストール済みの設定に対して実行してから、root の helper をアンロードします。既存の設定、インポートしたソース、ポリシーのデータ、実行履歴は保持されます。`config.yaml` がパッケージに含まれる例から作成されるのは、初回インストール時だけです。

## 透過プロキシと下流 IPv6 の引き継ぎ

OpenSurge は、入口の仕組みが異なる一方で mihomo のルールとアウトバウンドを共有する、2 つの透過データ経路を使用します。下流の IPv4 とローカルの Mac の透過プロキシは mihomo の TUN のメインラインを使用し、実験的な下流 IPv6 は macOS の BPF パケットブローカーと、OpenSurge でパッチを当てた `opensurge-packet` リスナーを通じて mihomo の gVisor に入ります。

どちらの経路も、ゲートウェイ全体の前提条件として `transparent.mode: "tun"` が必要ですが、下流の IPv6 のトラフィックは macOS のシステムの utun には入りません。これによって `redir-port` や PF の TCP リダイレクトが再び有効になることはありません。

### mihomo の TUN メインライン

下流の IPv4 とローカルの Mac の透過プロキシは mihomo の TUN を使用します。現在の Darwin ビルドでは実行時に redir が非対応と報告されるため、mihomo の `redir-port` と PF の TCP リダイレクトは意図的に非対応のままです。`mihomo.redir_port` と `pf.redirect_tcp_to` は `0` のままにしてください。

OpenSurge は、起動前に既存の utun インターフェイスや公開ルートとの競合を予測しません。mihomo が TUN のランタイムの準備完了を報告するまで待ちます。失敗した場合は、プロセスに短いクリーンアップの猶予を与え、ゲートウェイのランタイムをロールバックし、実際の TUN のエラーに、選択されたルートのインターフェイスとゲートウェイの情報を付け加えます。デフォルトでは、フルルートの TUN を 2 つ同時に使うことはサポートされていません。

Mac の IPv6 の TUN ルーティングは、下流の引き継ぎとは独立して動作します。TUN は公開 IPv6 とフェイク IPv6 のプール全体をキャプチャします。`dns.ipv6` は AAAA の応答だけを制御します。`local_system_dns.enabled` はデフォルトで true で、TUN の自動ルートの準備ができた後に上流のネットワークサービスの DNS を管理します。停止時とエンジンの再起動時には、保存した元の DNS を復元し、引き継ぎ中に他のソフトウェアやユーザーが行った変更を上書きします。元のネットワークサービスが削除されている場合、復旧ではそれをスキップし、同じ名前の新しいサービスは変更しません。復旧に失敗した場合は警告を出し、再試行用に独立したバックアップを保持したまま、ゲートウェイの他のクリーンアップを続けます。Web GUI の「Mac system DNS」の設定で無効にできます。インポートするプロファイルでは、明示的な DNS リゾルバーを使用する必要があります。`system` はリゾルバーのループを引き起こすため拒否されます。

### 下流 IPv6 の引き継ぎ（実験的）

Network ページには、独立した 2 つの設定があります。`dns.ipv6` は、OpenSurge の DNS が AAAA クエリに応答してフェイク IPv6 アドレスを作成するかどうかを決めます。`transparent.tun_ipv6` は、下流の IPv6 ゲートウェイ、SLAAC/RDNSS、ユーザー空間のパケット経路を制御し、`off`、`auto`、`always` のモードがあります。`auto` は、上流のインターフェイスに公開のグローバル IPv6 アドレス（ULA は公開の到達性に含まれません）と IPv6 のデフォルトルートの両方がある場合にのみ有効になります。`always` は、上流にネイティブの IPv6 がなくても下流の経路を確立します。

3 つの構成はいずれも `transparent.mode: "tun"` が必要です。隔離された下流 LAN では、RA/SLAAC/RDNSS を自動で配信します。同一 LAN の DHCP 引き継ぎでも、メインルーターの IPv6 の RA/DHCPv6 を無効にし（または RA Guard によって OpenSurge だけがデフォルトルーターを提供するようにし）、`transparent.ipv6_shared_l2_ready: true` を確認すれば、LAN 全体に対して同じことができます。どちらの自動モードも、標準の Medium の RA ルーター優先度を使用します。バイパスルーターモードでは RA を送信しません。OpenSurge の ULA、Mac のリンクローカルのデフォルトゲートウェイと DNS アドレスを手動で設定し、競合するルーターのデフォルトルートがないクライアントだけが登録されます。Network ページには、IPv4/IPv6 の入力用カードが表示されます。

この IPv6 データプレーンは、OpenSurge が mihomo に追加した `opensurge-packet` リスナーに依存しています。OpenSurge のパッケージに含まれる mihomo のバイナリは、[`patches/mihomo`](patches/mihomo/) にあるパケットリスナーのパッチを適用したうえで、固定した上流のソースからビルドされています。変更されていない上流のバイナリではありません。正確なバージョン、ライセンス、上流のソースは [Third-Party Notices](THIRD_PARTY_NOTICES.md) に記載しています。

下流の IPv6 のトラフィックは macOS のシステムの utun には入りません。macOS の BPF ブローカーが物理的な Ethernet フレームから IPv6 の L3 パケットと送信元の MAC アドレスを読み取り、モード `0600` の Unix データグラムを通じて `opensurge-packet` リスナーに渡します。リスナーはパケットを mihomo の gVisor に注入し、MAC アドレスを `IN-USER(device:<id>)` に対応付けて、既存のデバイスのルールとアウトバウンドを再利用します。戻りのパケットはブローカーを経由して下流の Ethernet に戻ります。この経路は TCP と UDP に対応しています。QUIC は UDP/443 で運ばれます。任意の IPv6 プロトコルをプロキシできるという意味ではありません。

DHCP 引き継ぎでのデバイスごとの **Direct via main router** は、IPv4 だけをバイパスします。下流の IPv6 が有効な場合、そのデバイスから OpenSurge のパケット経路に届いた IPv6 には最優先の `REJECT` が適用されます。他のデバイスは通常の IPv6 のポリシーのままです。クライアントは SLAAC のアドレスや RDNSS を保持し続ける可能性があるため、UI では IPv6 がないとは示さず、**IPv6 egress blocked** と表示します。メインルーターがまだ RA を配信している場合、クライアントは IPv6 で OpenSurge を迂回できてしまいます。メインルーターの RA/DHCPv6 を無効にするか、RA Guard を適用してください。

`always` は、公開 IPv6 の接続性を作り出すものではありません。上流にネイティブの IPv6 がない場合でも、フェイク IPv6 の宛先には必要なトラフィックに対応したプロキシを使用できますが、実際の公開 IPv6 アドレスへの `DIRECT` には上流のルートがありません。HTTP 専用のプロキシでは UDP/QUIC を運べません。

## mihomo のプロファイル

OpenSurge for Mac は、管理対象の mihomo の設定を生成することも、既存の mihomo のプロファイルをインポートすることもできます。インポートモードでも、LAN へのバインド、`allow-lan`、DNS リスナー/フェイク IP の範囲、TUN、`external-controller`、ランタイムのパスなど、ゲートウェイにとって重要なフィールドは引き続き OpenSurge が管理します。インポートしたプロファイルからは、`hosts`、`proxies`、`proxy-providers`、`proxy-groups`、`rule-providers`、`rules` と、ゲートウェイ以外の DNS リゾルバー/フィルターのフィールドが取り込まれます。`nameserver-policy`、`proxy-server-nameserver`、`fake-ip-filter` などのフィールドを保持することで、プロファイルがゲートウェイの DNS リスナーや TUN の DNS の取り決めを置き換えることなく、プロキシサーバーのホスト名を解決できるようにしています。

`hosts` の IP マッピング、ドメインのエイリアス、ワイルドカードのマッピングは、最終的な設定に保持されます。元のローカルの DNS リスナーを指すリゾルバーのアドレスは、自動では調整されません。hosts を保持しても、古い待ち受けポートへの依存は解消されません。

高度な設定のためにプロファイルをインポートする必要はありません。Web GUI の **Global Profile Overlay** では、最小限の管理対象プロファイルに、プロキシ、プロバイダー、手動のポリシーグループ、ルールを独立して追加できます。ゲートウェイが停止している間、Policies ページは準備済みの mihomo のインスタンスを使い、TUN、DHCP/DNS、PF、フォワーディングを引き継がずに、最終的な設定の表示、ノードの選択、レイテンシのテストを行います。先に Policies を開かずに Web GUI から直接起動することもできます。サーバーが設定を合成し、実際の `mihomo -t` で検証してから、その同じ候補を保存して起動します。

プレビュー、ノードの選択、レイテンシのテストを行っても、目的の設定は確定しません。ゲートウェイの実行中にオーバーレイの下書きを保存しても、ライブのトラフィックは変わりません。オーバーレイだけの下書きは、次に Web GUI からゲートウェイを起動したときに反映されます。`sudo omg start` は引き続き永続化された設定を使い、Web GUI の下書きは読み込みません。

```yaml
mihomo:
  profile_mode: "imported"
  profile: "./profiles/home.yaml"
  store_fake_ip: true
```

相対パスの `mihomo.profile` は、OpenSurge の設定ファイルのディレクトリを基準に解決されます。インポートした `proxy-providers` と `rule-providers` 内の相対パスの `path:` は、インポートした mihomo のプロファイルのディレクトリを基準に解決されます。OpenSurge は `profile.store-selected: true` を出力するため、mihomo は再起動後もポリシーグループの選択を保持できます。デフォルトの `mihomo.store_fake_ip: true` は `profile.store-fake-ip: true` を出力し、適用/再起動の後に既存のフェイク IP のマッピングを復元します。ゲートウェイの停止中に **Advanced Mihomo / DNS settings** で無効にできますが、その場合、長時間動作しているプロセスがキャッシュしたフェイク IP は、mihomo の再起動後に古くなる可能性があります。

ゲートウェイのサービスを起動する前に、最終的に生成される mihomo の設定をプレビューします。

```sh
go run ./cmd/omg doctor --config examples/config.imported-profile.example.yaml
go run ./cmd/omg render-mihomo --config examples/config.example.yaml
go run ./cmd/omg render-mihomo --config examples/config.imported-profile.example.yaml
```

`mihomo.binary` がインストール済みの mihomo のバイナリを指している場合は、`validate-mihomo` を使用します。最終的な設定を生成し、ゲートウェイのサービスを起動せずに mihomo 自身の `-t` による検証を実行します。

```sh
go run ./cmd/omg validate-mihomo --config examples/config.imported-profile.example.yaml
```

## CLI の使い方

これらのコマンドは、開発、自動化、診断を目的としています。パッケージを利用している場合は、[アプリユーザーガイド](docs/user/app-user-guide.md) のグラフィカルな手順に従ってください。

### 状態と診断

```sh
go run ./cmd/omg doctor --config examples/config.example.yaml
go run ./cmd/omg status --config examples/config.example.yaml
go run ./cmd/omg status --config examples/config.example.yaml --format json
go run ./cmd/omg logs --config examples/config.example.yaml --tail 50 --format json
go run ./cmd/omg snapshot --config examples/config.example.yaml --tail 50 --format json
```

### ポリシー、デバイス、プロバイダー

```sh
go run ./cmd/omg policies --config examples/config.imported-profile.example.yaml
go run ./cmd/omg policy-select \
  --config examples/config.imported-profile.example.yaml \
  --group Proxy \
  --policy DIRECT

# Local-Mac Rule / Global / Direct (downstream devices remain unchanged):
go run ./cmd/omg local-routing \
  --config examples/config.imported-profile.example.yaml
go run ./cmd/omg local-routing-set \
  --config examples/config.imported-profile.example.yaml \
  --mode global \
  --policy Proxy

# After configuring device_policy.file:
go run ./cmd/omg devices --config ./config.yaml --format json
go run ./cmd/omg device-policy-select \
  --config ./config.yaml \
  --device alice-phone \
  --slot default \
  --policy DIRECT

go run ./cmd/omg connections \
  --config examples/config.imported-profile.example.yaml \
  --format json
go run ./cmd/omg providers \
  --config examples/config.imported-profile.example.yaml \
  --format json
go run ./cmd/omg provider-update \
  --config examples/config.imported-profile.example.yaml \
  --provider demo-provider \
  --format json
```

### 設定の生成

```sh
go run ./cmd/omg render-mihomo --config examples/config.example.yaml
go run ./cmd/omg validate-mihomo \
  --config examples/config.imported-profile.example.yaml
```

### ゲートウェイのライフサイクル

次のコマンドは DHCP、DNS、PF、IPv4 フォワーディング、または mihomo のランタイムの状態を変更するため、`sudo` が必要です。

```sh
sudo go run ./cmd/omg start --config examples/config.example.yaml --format json
sudo go run ./cmd/omg reload --config examples/config.example.yaml --format json
sudo go run ./cmd/omg restart-mihomo --config examples/config.example.yaml --format json
sudo go run ./cmd/omg stop --config examples/config.example.yaml --format json
```

その他の挙動:

- `policy-select` はライブの mihomo のポリシーグループを読み取り、選択の変更を送信する前に、不明なグループやポリシーを拒否します。
- `provider-update --provider <name>` は、選択したプロキシプロバイダーの更新を mihomo に依頼し、更新後の状態を返します。
- `logs --tail N --format json` は、dnsmasq と mihomo の最近のログ行を、ファイルごとの存在と読み取りエラーのフィールド付きで返します。
- `snapshot --format json` は、status、doctor のチェック、リース、ログ、ポリシーグループ、接続、プロバイダーを集約します。mihomo の API が失敗しても、スナップショットの残りの部分は返されます。
- `restart-mihomo` はプロキシエンジンだけを再起動します。dnsmasq の停止、PF のアンロード、IPv4 フォワーディングの復元、ホストのネットワーク設定の変更は行いません。
- `--format json` はゼロ以外の失敗時の終了コードを維持し、構造化されたエラーを stderr に出力します。`start` と `stop` が成功した場合、ペイロードに `command`、`ok`、`config_path` が含まれます。

<a id="an-ai-agent-friendly-engineering-workspace"></a>
## AI エージェントにやさしいエンジニアリングワークスペース

OpenSurge は、リポジトリを単にコードを置く場所ではなく、エンジニアリングシステムの一部として扱います。目標は、プロダクトの意図、ネットワークの安全ルール、実行時の証拠、蓄積されたプロジェクトの知識を、人間のコントリビューターとコーディングエージェントの両方が直接読み取れるようにすることです。

### Harness Engineering: エージェントを取り巻く環境を設計する

このワークスペースは、[Harness Engineering](https://openai.com/index/harness-engineering/) の実践的な考え方を取り入れています。エージェントの作業の信頼性は、モデルを取り巻くコンテキスト、制約、ツール、可観測性、受け入れゲートによって決まる、という考え方です。

- `AGENTS.md` はコンパクトな入口のマップです。プロダクトのアイデンティティ、ネットワークに関する厳格な不変条件、タスクごとにエージェントが読むべき詳細なドキュメントを示しています。
- [`docs/agent-wiki/`](docs/agent-wiki/README.md) は、アーキテクチャ、意思決定、検証のコンテキストを段階的に開示します。タスクのたびにリポジトリを一から調べ直す必要はありません。
- `status`、`doctor`、`logs`、`snapshot` などの機械可読な CLI、決定的な `make` ターゲット、保持される成果物によって、実行中のシステムをエージェントが観測できるようにしています。
- 設定の検証、TUN 専用の透過ルーティング、ロールバックの挙動、隔離されたラボ、明示的な復旧の取り決めによって、安全に関するガイダンスを強制力のある境界に変えています。

### Loop Engineering: 実行可能な証拠でループを閉じる

OpenSurge は、[Loop Engineering](https://addyosmani.com/blog/loop-engineering/) の中核となる考え方に従っています。1 つの巧妙なプロンプトに頼るのではなく、行動、観測、検証、復旧を行い、その結果を次のイテレーションに引き継げる、繰り返し可能なシステムを設計します。

```text
intent + constraints
        ↓
AGENTS.md → Agent Wiki → source of truth
        ↓
implement → fast tests → Virtual LAN Lab
        ↓
ADB-assisted or manual real-device validation
        ↓
logs + artifacts + cleanup/recovery proof
        ↓
durable learning returns to sources/ and wiki/
        ↺
```

検証の各レイヤーは互いに補完し合います。

- `make test` と、対象を絞った UI/コントロールプレーンのゲートが、高速な内側のループを担います。
- Lima + socket_vmnet の Virtual LAN Lab によって、特権が必要な DHCP、DNS、pf/NAT、フォワーディング、TUN、ポリシー、ロールバック、クリーンアップの挙動を、通常の LAN を危険にさらさずに再現できます。
- 実機と、同一 LAN/同一 Wi-Fi のランナーが、物理的な構成のループを閉じます。ADB で Android のルート、DNS、接続性の証拠を収集し、Mac 側で dnsmasq/mihomo のログと照合できます。運用者が直接の制御を保ちたい場合は、スマートフォンでの手動のチェックポイントも引き続きサポートしています。
- リスクのある DHCP 引き継ぎの手順では、復旧も受け入れ条件の一部です。トラフィックのプローブが成功しただけでは、ルーター、Mac、クライアントを既知の正常な状態に戻せない場合は十分ではありません。

Virtual Lab の結果は実機での挙動の代わりにはならず、1 回の物理的なスモークテストが決定的な Lab のゲートの代わりになることもありません。各ゲートが裏付けてよい主張の正確な範囲については、[検証の取り決め](docs/agent-wiki/sources/validation/test-gates.md) を参照してください。

### Agent Wiki: 外部化されたプロジェクトの記憶

[Agent Wiki](docs/agent-wiki/wiki/index.md) は、一時的なコンテキストウィンドウから永続的な記憶を取り出し、小さくバージョン管理された、ソースに裏付けられた知識レイヤーに移すという LLM Wiki の考え方を取り入れています。

- `docs/agent-wiki/sources/` では、プロジェクトの仕様、ドメインの取り決め、検証の方法、範囲を限定した証拠を管理しています。
- `docs/agent-wiki/wiki/concepts/` では、概念、関係、責任を短い文章と任意の図で説明し、関連するソースにリンクしています。答えが分かっている質問は、直接ソースやコードを参照できます。
- `docs/user/` には、運用者向けのインストール、設定、使い方のガイドをまとめています。
- `.codex/hooks.json` は、インストールされている場合にローカルの Session Wiki のフックを組み込みます。これにより、非公開のセッションの状態をコミットすることなく、セッションの継続とコンパクションでプロジェクトローカルの記憶を使えます。

現在の取り決め、検証の方法、過去の結果には、それぞれの適用範囲が明記されています。以前の記録は、現在のチェックアウトの受け入れを保証するものではありません。1 回限りの完全なログ、一時的なアイデア、通常の TODO は、現在の作業コンテキストにとどめます。完了済みの既存のタスクと実行レポートは `docs/agent-wiki/tasks/finished_archived/` に移され、`.ignore` によってデフォルトの検索から除外されます。検証のソースでは、テストの実行ごとにページを追加するのではなく、再利用できる方法と必要な証拠の要約を管理しています。

## ライセンス

個別の表示がない OpenSurge for Mac のオリジナルのコードとアセットは、[GNU General Public License version 3 only](LICENSE)（`GPL-3.0-only`）のもとでライセンスされています。同梱されているサードパーティのプログラムとライブラリは、それぞれのライセンスに従います。同梱されている mihomo と dnsmasq のバージョンに対応するソースへの正確なリンクを含め、[Third-Party Notices](THIRD_PARTY_NOTICES.md) を参照してください。

## 安全性

`start` と `stop` は、DHCP、pf、IPv4 フォワーディングを管理するため、`sudo` で実行することを想定しています。ランタイムのファイルは、設定ファイルの `runtime.dir` の下に書き込まれます。

## 開発ワークフロー

高速なデフォルトのゲートとして `make test` を使用します。CI は現在このユニットテストのゲートだけを実行するため、通常の push やプルリクエストに、ホストのネットワーク、パスワードなしの sudo、Lima、socket_vmnet は必要ありません。

リスクの高いネットワークの変更をコミットまたはレビューする前に、ローカルで `make lab-test` を実行してください。対象には、DHCP、DNS、mihomo の起動/設定の生成、pf のルール、フォワーディング/ロールバックの挙動、ゲートウェイのライフサイクル、ラボのスクリプト、実行時のトラフィックに影響する設定例の変更が含まれます。同じ制御されたホスト権限とネットワークの隔離を提供できる専用の macOS ランナーが用意されるまで、仮想 LAN ラボはローカル、夜間、または手動のゲートとして運用します。

サポートされている透過プロキシの経路には `make lab-test-tun` を使用します。このテストではクライアントをプロキシなしの状態に保ち、mihomo が TUN のインバウンドを通じた直接の HTTPS 接続をログに記録することを確認します。mihomo のプロファイルのインポートやオーバーレイの挙動を変更する場合は `make lab-test-tun-imported-profile` を使用します。インポートしたプロファイルのフィクスチャを使って、同じ TUN のゲートを実行します。透過的な TUN のトラフィックに影響するはずの、インポートしたプロバイダーやポリシーの選択の挙動を変更する場合は `make lab-test-tun-imported-egress` を使用します。ローカルの HTTP プロバイダーと制御された HTTP CONNECT プロキシを使い、`policy-select` によって TUN の出口経路が `DIRECT` と制御されたプロキシの間で切り替わることを確認します。

ローカルの Mac の Rule/Global/Direct の selector や、ローカルと下流の隔離の境界を変更する場合は、`make lab-test-tun-local-routing` を使用します。両方向を確認します。ローカルを Global にすると、下流のクライアントは直接のゲートウェイのルールのままで制御されたプロキシを使えること、ローカルを Direct にすると、下流のゲートウェイのルールがまだプロキシを使っている状態でプロキシを迂回できることです。

MAC アドレスの予約、デバイスごとの selector、デバイスのオーバーライドのデータ経路を変更する場合は、`make lab-test-tun-device-policy` を使用します。2 台のクライアントがそれぞれ固定のリースを受け取り、異なる TUN の出口経路を独立して選択でき、デバイス単位のドメインの `REJECT` が適用されることを確認します。ドメイン/プロトコルのルールのコンパイル、テンプレート、HTTP/MRS のルールプロバイダーの設定はユニットテストでカバーされているため、運用者が定義したルールごとに Lab を実行する必要はありません。

Tailscale の宛先/送信元のルール、MagicDNS、managed-tsnet の設定、ネイティブアプリの検出を変更する場合は、`make lab-test-tailscale` を使用します。専用の Lima の Tailnet ピアで、正確なピア IP への TCP/UDP、完全な MagicDNS 名、Control API による検出、1 台のデバイスへの承認、もう 1 台のデバイスでの fail-closed の挙動を検証します。ピア側で観測された送信元アドレスによって、Mac のネイティブの Tailscale のルートを経由した誤検知も排除します。初回の実行には、ピアと管理対象のノード用に、外部のモード `0600` の auth-key ファイルが必要です。1 回限りのキーを 2 つ用意するか、両方のファイル変数で共有する、再利用可能で Ephemeral ではないキーを 1 つ用意します。ID が永続化されるため、通常の再実行ではキーは不要です。この範囲を限定したゲートは、サブネットルーター、Exit Node、Headscale、実際のリモート LAN を証明するものではありません。

下流の RA/SLAAC、BPF ブローカー、パッチを当てた mihomo のパケットリスナー、IPv6 のデバイスの識別、停止時の取り下げを変更する場合は、構成ごとのゲート `make lab-test-ipv6-userspace`、`make lab-test-ipv6-same-wifi`、`make lab-test-ipv6-same-lan` を実行します。RA を自動で配信するゲートでは、2 台のクライアントが OpenSurge の IPv6 アドレス、Medium 優先度のデフォルトルート、リンクローカルの DNS を取得する必要があります。バイパスルーターのゲートでは、手動の ULA、Mac のリンクローカルのデフォルトゲートウェイと DNS が必要で、RA がないことも求められます。3 つとも、制御されたローカルのフィクスチャを使って、TCP、UDP のリクエスト/レスポンス、QUIC 形式の UDP キャリア、TCP や HTTP/2 へのフォールバックがない実際の HTTP/3 専用のリクエスト/レスポンスを検証します。HTTP/3 は、`DIRECT`、制御された UDP 対応の SOCKS5 アウトバウンド、HTTP 専用で fail-closed のアウトバウンドを通じて確認し、あわせてデバイスごとのポリシー、双方向の BPF の証拠、ロールバックも確認します。この範囲を限定したゲートは、あらゆる QUIC/HTTP3 の実装、バージョン、コネクションマイグレーションのケース、公開プロキシの組み合わせをカバーするものではありません。

ポリシーの制御や機械可読な CLI の変更には、`make policy-control-test` を使用します。sudo、dnsmasq、pf、TUN なしで実際の mihomo のバイナリを起動し、ライブの external-controller の API に対して、`policies`、無効および有効な `policy-select`、mihomo の再起動後に永続化された選択の復元、mihomo の mixed-port を通じたローカル/プライベートの `DIRECT` のガード、専用のローカルルーティングのモードコントローラー、`connections`、`providers`、ファイルと HTTP のプロキシプロバイダーに対する `provider-update`、`snapshot` を確認します。

同一 LAN のデフォルトゲートウェイに限定したスモークテストには、`make same-lan-start-tun` と `make same-lan-adb-check` を使用します。このゲートでは DHCP を無効のまま TUN を必須とし、ゲートウェイと DNS を Mac の LAN IP に向けた 1 台の Android のテストデバイスを ADB で調べます。完全なサブスクリプションをインポートする前に、`api.ipify.org` のような 1 つのドメインで実際のプロキシの出口を確認するには、`OMG_SAME_LAN_*` の上流プロキシ用の環境変数のオーバーライドとともに `make same-lan-start-tun-proxy` を使用します。より実機に近いスモークテストには、`make same-lan-start-tun-imported-egress` と `make same-lan-adb-check-imported-egress` を使用します。プロバイダーに基づく `TunEgress` グループをインポートし、同一 LAN の TUN のトラフィックを `DIRECT` から制御されたローカルの HTTP CONNECT プロキシに切り替えます。これらのゲートは、LAN 全体への展開の準備ができていることや、実際のリモートのサブスクリプションの出口を主張するものではありません。意図的に ADB を使えないようにしている場合は、Android のブラウザでの手動のプローブで、同じインポートした出口の証拠を収集できます。[`tests/same-lan/README.md`](tests/same-lan/README.md#manual-phone-check-without-adb) を参照してください。

ルーターの DHCP を手動で無効にした専用のテスト用 Wi-Fi では、`make same-wifi-dhcp-start-imported-egress` を使用した後、Android のクライアントを DHCP モードで再接続し、`make same-wifi-dhcp-adb-check-imported-egress` を実行します。この別のリスクの高いランナーは `gateway.mode: "same_wifi_dhcp"` を使用し、保護する静的アドレスの明示的な一覧と、ルーターの DHCP が無効になっていることの運用者による確認が必要です。停止時のゲートでは OpenSurge のクリーンアップを検証しますが、ルーターの DHCP とクライアントの自動アドレス設定は手動で元に戻す必要があります。[`tests/same-lan/WIFI-DHCP-RUNNER.md`](tests/same-lan/WIFI-DHCP-RUNNER.md) を参照してください。

## 仮想 LAN ラボ

統合ラボでは、実際の macOS のゲートウェイを 2 台の軽量な Linux クライアントに対して実行します。Lima がクライアントを提供し、socket_vmnet が競合する DHCP サーバーのない隔離されたレイヤー 2 のホストネットワークを作成します。テストでは、DHCP、DNS、ICMP/NAT、直接の HTTPS、mihomo の `mixed-port` を通じた明示的な HTTPS をカバーします。

```sh
make lab-install
make lab-up
sudo -v
make lab-test
make lab-test-tun
make lab-test-tun-imported-profile
make lab-test-tun-imported-egress
make lab-test-tun-device-policy
make lab-test-tailscale
make lab-test-ipv6-userspace
make lab-test-ipv6-same-wifi
make lab-test-ipv6-same-lan
make lab-down
```

1 回限りのインストーラーは、root が所有する機能固定のネットワークヘルパーと、ラボのネットワークの起動、停止、確認のための限定的な sudoers のルールを追加します。ゲートウェイのバイナリ自体には、パスワードなしの root アクセスは付与されません。エンドツーエンドのテストの前に、`sudo -v` で sudo のチケットを更新してください。構成、安全性のチェック、トラブルシューティングの手順については、`tests/lab/README.md` を参照してください。
