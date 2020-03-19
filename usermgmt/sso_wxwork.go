/*
 * @Copyright Reserved By Janusec (https://www.janusec.com/).
 * @Author: U2
 * @Date: 2020-03-14 09:58:15
 * @Last Modified: U2, 2020-03-14 09:58:15
 */

package usermgmt

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/Janusec/janusec/data"
	"github.com/Janusec/janusec/models"
	"github.com/Janusec/janusec/utils"
	"github.com/gorilla/sessions"

	"github.com/patrickmn/go-cache"
)

var (
	OAuthCache = cache.New(5*time.Minute, 5*time.Minute)
)

type WxworkAccessToken struct {
	ErrCode     int64  `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type WxworkUser struct {
	ErrCode int64  `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
	UserID  string `json:"UserId"`
}

// https://work.weixin.qq.com/api/doc/90000/90135/91025
// Step 1: To https://open.work.weixin.qq.com/wwopen/sso/qrConnect?appid=CORPID&agentid=AGENTID&redirect_uri=REDIRECT_URI&state=admin
// If state==admin, for janusec-admin; else for frontend applications
func CallbackWithCode(w http.ResponseWriter, r *http.Request) (*models.AuthUser, error) {
	// Step 2.1: Callback with code, http://gate.janusec.com/?code=BM8k8U6RwtQtNY&state=admin&appid=wwd03ba1f8
	code := r.FormValue("code")
	state := r.FormValue("state")
	// Step 2.2: Within Callback, get access_token
	// https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=wwd03ba1f8&corpsecret=NdZI
	// Response format: https://work.weixin.qq.com/api/doc/90000/90135/91039
	accessTokenURL := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s",
		data.CFG.MasterNode.Wxwork.CorpID, data.CFG.MasterNode.Wxwork.CorpSecret)
	request, _ := http.NewRequest("GET", accessTokenURL, nil)
	resp, _ := GetResponse(request)
	tokenResponse := WxworkAccessToken{}
	json.Unmarshal(resp, &tokenResponse)
	// Step 2.3: Get UserID, https://qyapi.weixin.qq.com/cgi-bin/user/getuserinfo?access_token=ACCESS_TOKEN&code=CODE
	userURL := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/user/getuserinfo?access_token=%s&code=%s", tokenResponse.AccessToken, code)
	request, _ = http.NewRequest("GET", userURL, nil)
	resp, _ = GetResponse(request)
	wxworkUser := WxworkUser{}
	json.Unmarshal(resp, &wxworkUser)
	if state == "admin" {
		// Insert into db if not existed
		id, _ := data.DAL.InsertIfNotExistsAppUser(wxworkUser.UserID, "", "", "", false, false, false, false)
		// create session
		authUser := &models.AuthUser{
			UserID:        id,
			Username:      wxworkUser.UserID,
			Logged:        true,
			IsSuperAdmin:  false,
			IsCertAdmin:   false,
			IsAppAdmin:    false,
			NeedModifyPWD: false}
		session, _ := store.Get(r, "sessionid")
		session.Values["authuser"] = authUser
		session.Options = &sessions.Options{Path: "/janusec-admin/", MaxAge: tokenResponse.ExpiresIn}
		session.Save(r, w)
		return authUser, nil
	}
	// Gateway OAuth for employees and internal application
	oauthStateI, found := OAuthCache.Get(state)
	if found {
		oauthState := oauthStateI.(models.OAuthState)
		oauthState.Username = wxworkUser.UserID
		OAuthCache.Set(state, oauthState, cache.DefaultExpiration)
		//fmt.Println("1008 set cache state=", oauthState, "307 to:", oauthState.CallbackURL)
		http.Redirect(w, r, oauthState.CallbackURL, http.StatusTemporaryRedirect)
		return nil, nil
	}
	//fmt.Println("1009 Time expired")
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	return nil, nil
}

func GetResponse(request *http.Request) (respBytes []byte, err error) {
	request.Header.Set("Accept", "application/json")
	client := http.Client{}
	resp, err := client.Do(request)
	utils.CheckError("GetResponse Do", err)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBytes, err = ioutil.ReadAll(resp.Body)
	return respBytes, err
}

/*
func OAuth2Demo(ctx *gin.Context) {
	// This is a demo instead of Github
	ctx.Redirect(http.StatusFound, "/oauth2/callback/github?code=a9fe4d0d42cfbd2ba1d8")
}
*/