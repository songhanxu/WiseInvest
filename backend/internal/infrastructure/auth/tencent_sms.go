package auth

import (
	"context"
	"fmt"
	"os"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	sms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111"
)

type tencentSMSSender struct {
	client     *sms.Client
	sdkAppID   string
	signName   string
	templateID string
}

func NewTencentSMSSenderFromEnv() SMSSender {
	id := os.Getenv("TENCENTCLOUD_SECRET_ID")
	key := os.Getenv("TENCENTCLOUD_SECRET_KEY")
	appID := os.Getenv("TENCENT_SMS_SDK_APP_ID")
	sign := os.Getenv("TENCENT_SMS_SIGN_NAME")
	template := os.Getenv("TENCENT_SMS_TEMPLATE_ID")
	if id == "" || key == "" || appID == "" || sign == "" || template == "" {
		return nil
	}
	config := profile.NewClientProfile()
	config.HttpProfile.Endpoint = "sms.tencentcloudapi.com"
	config.HttpProfile.ReqTimeout = 8
	client, err := sms.NewClient(common.NewCredential(id, key), "ap-guangzhou", config)
	if err != nil {
		return nil
	}
	return &tencentSMSSender{client: client, sdkAppID: appID, signName: sign, templateID: template}
}

func (s *tencentSMSSender) Send(ctx context.Context, phone, code string) error {
	request := sms.NewSendSmsRequest()
	request.SmsSdkAppId = common.StringPtr(s.sdkAppID)
	request.SignName = common.StringPtr(s.signName)
	request.TemplateId = common.StringPtr(s.templateID)
	request.TemplateParamSet = common.StringPtrs([]string{code})
	request.PhoneNumberSet = common.StringPtrs([]string{"+86" + phone})
	response, err := s.client.SendSmsWithContext(ctx, request)
	if err != nil {
		return err
	}
	if response.Response == nil || len(response.Response.SendStatusSet) != 1 || response.Response.SendStatusSet[0].Code == nil || *response.Response.SendStatusSet[0].Code != "Ok" {
		return fmt.Errorf("tencent SMS rejected the message")
	}
	return nil
}
