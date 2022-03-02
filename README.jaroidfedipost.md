This CLI utility downloads videos from [nicovideo.jp](https://www.nicovideo.jp/) also optionally uploading them as
pleroma/mastodon instance posts, using OAuth2 API to authenticate under provided account.

USAGE
---

```
Usage:
  jaroidfedi https://www.nicovideo.jp/watch/sm0000000 <size[!]|formatid|max> [post|list|preview] [account]

Application Options:
  -f, --fediverse=  Fediverse instance URL
  -l, --login=      Fediverse instance login
  -d, --dir=        Download directory
  -o, --output=     Output file
  -c, --config=     Config file location
  -j, --cookie-jar= Cookie jar file
      --listen=     Listen for authorization code
  -u, --username=   Nicovideo username
  -p, --password=   Nicovideo password
  -q, --quiet       Suppress extra output
      --default     Set specifid url/login/args as default

Help Options:
  -h, --help             Show this help message

Available commands:
  account
  
[account command options]
          --code=   OAuth2 code
```

- To add an account 
  - Using localhost HTTP server
    ```sh
    ./jaroidfedi account -f your.instance.domain -l yourlogin --listen
    ```
    this will start local HTTP server on random port.
    To choose a specific port or host, you can use e.g. `--listen=:8080`
  - To add an account manually
    ```sh
    ./jaroidfedi account -f your.instance.domain -l yourlogin
    ```
  
  Either way, URL should be printed, which may look like

  https://your.instance.domain/oauth/authorize?client_id=wDQkkxM09VsWztb3XDwBOvHihqM6_RUkpx7K6vEFRIQ&redirect_uri=urn%3Aietf%3Awg%3Aoauth%3A2.0%3Aoob&response_type=code&scope=write%3Astatuses+write%3Amedia

  By following this URL, your insance' FE should display a dialog with list of scopes to grant to jaroid application.

  For posting statuses with video attachments, it needs two scopes
  `write:statuses` and
  `write:media`
  
  After granting access
  - If you started localhost HTTP
    
    You will be redirected to localhost server and account is aded.
  - If you choose manual code exchange
  
    FE should display a token, which then can be passed to cli as
    ```sh
    ./jaroidfedi account -f your.instance.domain -l yourlogin --code yourcode
    ```
  You only need to this once, for one account, unless you revoke this token (it should display as `jaroid`) in your account security options.
- To change default instace/account
  ```sh
  ./jaroidfedi -f your.instance.domain -l yourlogin --default
  ```
- You can list available formats with
  ```sh
  ./jaroidfedi https://www.nicovideo.jp/watch/sm0000000 list
  ```
- To download a video with selected format use
  ```sh
  ./jaroidfedi https://www.nicovideo.jp/watch/sm0000000 formatid
  ```
- Alternatively preselect a format with estimated filesize less or equal to desired
  ```sh
  ./jaroidfedi https://www.nicovideo.jp/watch/sm0000000 50m
  ```
- Or force smallest available, if there is no formats smaller or equal to selected
  ```sh
  ./jaroidfedi https://www.nicovideo.jp/watch/sm0000000 50m!
  ```
- Alternatively preselect a maximum available format
  ```sh
  ./jaroidfedi https://www.nicovideo.jp/watch/sm0000000 max
  ```
- To post a video, add 'post'
  ```sh
  ./jaroidfedi https://www.nicovideo.jp/watch/sm0000000 <size[!]|formatid|max> post
  ```
- To pass extra options to youtube-dl
  ```sh
  ./jaroidfedi https://www.nicovideo.jp/watch/sm0000000 -u nicovideologin -p nicovideopassword
  ```
- You can output fedi post markup to stdout without downloading and creating a post
  ```sh
  ./jaroidfedi https://www.nicovideo.jp/watch/sm0000000 preview
  ```

Config 
---

Config file is stored in in `~/.config/jaroid/fedi.yml` by default.

Example structure

```yaml
instances:
  your.instance.domain:
    url: https://your.instance.domain
    default_account: yourlogin
    endpoints:
      media: https://your.instance.domain/api/v1/media
      statuses: https://your.instance.domain/api/v1/statuses
      apps: https://your.instance.domain/api/v1/apps
      oauth_token: https://your.instance.domain/oauth/token
      oauth_authorize: https://your.instance.domain/oauth/authorize
    accounts:
      yourlogin:
        expire: 2089-10-15T01:08:05.480617676+03:00
        access_token: abababababababababababa
        refresh_token: odoruakachanningen
        type: Bearer
        scopes:
        - write:statuses
        - write:media
        redirect_uris:
          http://127.0.0.1:38080/callback: {}
          urn:ietf:wg:oauth:2.0:oob: {}
        redirect_uri: http://127.0.0.1:38080/callback
    clients:
      http://127.0.0.1:38080/callback:
        client_id: hitonomemitai
        client_secret: mirenakattari
        redirect_uri: http://127.0.0.1:38080/callback
        scopes:
          - write:statuses
          - write:media
global:
  template: |
    **{{.info.Title}}**
    {{.url}}
    {{- if .info.Tags.jp }}
    {{ .info.Tags.jp | makeTags | join " " }}
    {{- end }}
  user_agent: jaroid
  default_instance: your.instance.domain
mediaservice:
  save_dir: /tmp/jaroid
  cookie_jar: /home/user/.config/jaroid/cookie.jar
  keep_files: false
  auth:
    username: 
    password:
```

Template
---

Template uses [go template](https://pkg.go.dev/text/template) syntax.

Available extra functions:

|Function|Parameters|Returns|Description|
|---|---|---|---|
|`makeTag`|`string`|`string`|Makes a tag out of a string, removing unacceptable characters and prepending hashmark|
|`makeTags`|`[]string`|`[]string`|Applies makeTag to list of strings, returns list of tags|
|`join`|`delimiter string`, `ss []string`|`string`|joins `ss` using `delimiter`|

Available variables:

|Variable|Example|Descriptipn|
|---|---|---|
|`.url`|https://www.nicovideo.jp/watch/sm0000000|Raw nicovideo URL|
|`.file`|/tmp/jaroid/sm0000000-h264_360p_low-aac_64kbps.mp4|Local file path|
|`.filename`|sm0000000-h264_360p_low-aac_64kbps.mp4|Basename of local file|
|`.info`| |Thumbinfo data of video as returned by http://ext.nicovideo.jp/api/getthumbinfo/sm0000000 ; see https://site.nicovideo.jp/search-api-docs/search.html|
|`.info.FirstRetrieve`| |`first_retrieve` field|
|`.info.Tags`| {"jp": ["tag1", "tag2"], "en": ["tag1", "tag2"]} |Video tags by language|
|`.info.VideoID`| sm0000000 |Video ID|
|`.info.Title`| ABABABA |Video title|
|`.info.Description`| NINGEN ODORU |Video description|
|`.info.ThumbnailURL`| https://... |Video thumbnail url|
|`.info.MovieType`| mp4 |Video container|
|`.info.LastResBody`| HURRDURR |Latest comment body|
|`.info.WatchURL`| https://www.nicovideo.jp/watch/sm0000000 |Nicovideo URL|
|`.info.ThumbType`| video |Thumbnail type|
|`.info.Genre`| その他 |Video genre|
|`.info.UserID`| 000000 |User ID|
|`.info.UserNickname`| ずずずず |User nickname|
|`.info.UserIconURL`| https://... |User icon URL|
|`.info.Length`| 1h30m32s |Video duration|
|`.info.SizeHigh`| 1 |???|
|`.info.SizeLow`| 1 |???|
|`.info.ViewCounter`| 00000 |Video views|
|`.info.CommentNum`| 00000 |Video comments|
|`.info.MylistCounter`| 00000 |Video mylists|
|`.info.Embeddable`| true |If video can be embedded|
|`.info.NoLivePlay`| false |???|
