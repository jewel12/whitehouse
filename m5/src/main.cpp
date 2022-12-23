#include <stdio.h>
#include <string.h>

#include <M5EPD.h>
#include <WiFi.h>
#include <HTTPClient.h>
#include <ESP_Signer.h>
#include <ArduinoJson.h>
#include <env.h>

static const int JST = 3600 * 9;

using namespace std;
std::map<string, string> images;

const size_t capacity = 500;
DynamicJsonDocument doc(capacity);
DynamicJsonDocument filter_body(2048);

M5EPD_Canvas canvas(&M5.EPD);

SignerConfig config;

void token_status_callback(TokenInfo info);
void drive_files(void);

void setup()
{
    M5.begin();
    M5.EPD.Clear(true);

    WiFi.begin(WIFI_SSID, WIFI_PASS);
    while (WiFi.status() != WL_CONNECTED)
    {
        delay(500);
        Serial.print(".");
    }

    configTime(JST, 0, "ntp.nict.jp", "ntp.jst.mfeed.ad.jp");

    config.service_account.data.client_email = CLIENT_EMAIL;
    config.service_account.data.project_id = PROJECT_ID;
    config.service_account.data.private_key = PRIVATE_KEY;
    config.signer.tokens.scope = "https://www.googleapis.com/auth/drive, https://www.googleapis.com/auth/drive.file, https://www.googleapis.com/auth/drive.metadata";
    config.token_status_callback = token_status_callback;
    Signer.setSystemTime(time(NULL));
    Signer.begin(&config);
}

// ドライブから画像を取得する
int getPic(String image_id, uint8_t *&pic)
{
    WiFiClientSecure *client = new WiFiClientSecure;
    if (client)
    {
        client->setCACert(rootCA);
        {
            HTTPClient https;
            if (https.begin(*client, "https://www.googleapis.com/drive/v3/files/" + image_id + "?alt=media"))
            {
                https.addHeader("Authorization", "Bearer " + Signer.accessToken());
                int httpResponseCode = https.GET();
                size_t size = https.getSize();
                if (httpResponseCode == HTTP_CODE_OK || httpResponseCode == HTTP_CODE_MOVED_PERMANENTLY)
                {
                    WiFiClient *stream = https.getStreamPtr();
                    pic = (uint8_t *)ps_malloc(size);
                    size_t offset = 0;
                    while (https.connected())
                    {
                        size_t len = stream->available();
                        if (!len)
                        {
                            delay(1);
                            continue;
                        }
                        stream->readBytes(pic + offset, len);
                        offset += len;
                        if (offset == size)
                        {
                            break;
                        }
                    }
                }
                https.end();
                return size;
            }
            else
            {
                Serial.println("Connection failed");
            }
        }
    }
    client->stop();
}

void loop()
{
    bool ready = Signer.tokenReady();
    if (ready)
    {
        // Set expiration
        int t = Signer.getExpiredTimestamp() - config.signer.preRefreshSeconds - time(nullptr);

        // ドライブからファイル一覧の取得
        drive_files();

        // 画像の取得
        uint8_t *pic = nullptr;
        auto it = images.begin();
        advance(it, random(images.size()));
        String imageid = it->first.c_str();
        int size = getPic(imageid, pic);

        // 画像の描画
        canvas.createCanvas(960, 540);
        canvas.setTextSize(3);
        canvas.drawJpg(pic, size);
        canvas.pushCanvas(0, 0, UPDATE_MODE_GC16);
        free(pic);

        // Deep Sleep
        delay(120000);
    }
}

void token_status_callback(TokenInfo info)
{
    if (info.status == esp_signer_token_status_error)
    {
        Serial.printf("Token info: type = %s, status = %s\n", Signer.getTokenType(info).c_str(), Signer.getTokenStatus(info).c_str());
        Serial.printf("Token error: %s\n", Signer.getTokenError(info).c_str());
    }
    else
    {
        Serial.printf("Token info: type = %s, status = %s\n", Signer.getTokenType(info).c_str(), Signer.getTokenStatus(info).c_str());
        if (info.status == esp_signer_token_status_ready)
            Serial.printf("Token: %s\n", Signer.accessToken().c_str());
    }
}

string Str2chars(String Str)
{
    char buf[40];
    Str.toCharArray(buf, Str.length() + 1);
    return buf;
}

// ドライブのファイル一覧をimagesにかくのうする
void drive_files(void)
{
    WiFiClientSecure *client = new WiFiClientSecure;
    if (client)
    {
        client->setCACert(rootCA);
        HTTPClient https;
        Serial.println("HTTPS GET");

        String post_data = "?q='" + dirId + "'+in+parents";
        String url = "https://www.googleapis.com/drive/v3/files" + post_data;

        if (https.begin(*client, url))
        {
            https.addHeader("Content-Type", "application/json");
            String token = Signer.accessToken();
            https.addHeader("Authorization", "Bearer " + token);
            int httpResponseCode = https.GET();
            String body = https.getString();

            StaticJsonDocument<200> filter;
            filter["files"][0]["id"] = true;
            filter["files"][0]["name"] = true;
            deserializeJson(filter_body, body, DeserializationOption::Filter(filter));
            serializeJsonPretty(filter_body, Serial);

            int i = 0;
            while (1)
            {
                String temp_id = filter_body["files"][i]["id"];
                String temp_name = filter_body["files"][i]["name"];
                if (temp_id.equals("null"))
                {
                    break;
                }
                images[Str2chars(temp_id)] = Str2chars(temp_name);
                i++;
            }
            https.end();
        }
        else
        {
            Serial.println("Connection failed");
        }
    }
    client->stop();
}
