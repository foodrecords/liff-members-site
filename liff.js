$(document).ready(function () {
    initializeLiff(window.APP_CONFIG.liffId);
})

function initializeLiff(liffId) {
    liff
        .init({
            liffId: liffId
        })
        .then(() => {
            if (!liff.isInClient() && !liff.isLoggedIn()) {
                window.alert("LINEアカウントでログインするか、LINEアプリから開いてください。");
                liff.login({redirectUri: location.href});
            }else{
                const accessToken = liff.getAccessToken();
                if (accessToken) {
                    showPoint(accessToken);
                    var code = getParam('code');
                    if (code) {
                        checkCode(accessToken, code);
                    }
                }
            }
        })
        .catch((err) => {
            window.alert('LIFF Initialization failed: ' + err);
        });
}

function scanQR() {
    console.log('scan');
    liff
        .scanCodeV2()
        .then((result) => {
            console.log(result.value);
            var code = getParam('code', result.value);
            if (code) {
                const accessToken = liff.getAccessToken();
                if (accessToken) {
                    checkCode(accessToken, code);
                }
            }
        })
        .catch((err) => {
            console.log(err);
        });
}

function showPoint(token) {
    var apiurl = window.APP_CONFIG.apiUrl;
    console.log('[API] GET ' + apiurl + '/members');
    $.ajax({
        beforeSend: function(request) {
            request.setRequestHeader('Authorization', 'Bearer '+token);
        },
        dataType: "json",
        url: apiurl + '/members',
        success: function(data) {
            console.log('[API] /members response:', JSON.stringify(data));
            if (data.data) {
                $('#point-card-balance span').text(data.data.point);
                $('#point-card-number span').text(data.data.number);
            } else {
                $('#point').text('エラー');
            }
        },
        error: function (jqXHR) {
            var msg = jqXHR.responseJSON && jqXHR.responseJSON.message || jqXHR.statusText || 'network error (status=' + jqXHR.status + ')';
            console.error('[API] /members error:', jqXHR.status, msg);
            alert(msg);
        }
    });
}

function checkCode(token, code) {
    var apiurl = window.APP_CONFIG.apiUrl;
    console.log('[API] POST ' + apiurl + '/qrcode, code=' + code);
    $.ajax({
        beforeSend: function(request) {
            request.setRequestHeader('Authorization', 'Bearer '+token);
        },
        dataType: "json",
        url: apiurl + '/qrcode',
        type: 'post',
        data: JSON.stringify({ code: code }),
        success: function(data) {
            console.log('[API] /qrcode response:', JSON.stringify(data));
            if (data.data) {
                $('#point-card-balance span').text(data.data.point);
                $('#point-card-number span').text(data.data.number);
                if (data.data.get_point) {
                    $('#point-card-get').text(data.data.get_point + ' point get!').css('visibility','visible');
                }
            } else {
                $('#point').text('エラー');
            }
        },
        error: function (jqXHR) {
            var msg = jqXHR.responseJSON && jqXHR.responseJSON.message || jqXHR.statusText || 'network error (status=' + jqXHR.status + ')';
            console.error('[API] /qrcode error:', jqXHR.status, msg);
            alert(msg);
        }
    });
}


function getParam(name, url) {
    if (!url) url = window.location.href;
    name = name.replace(/[\[\]]/g, "\\$&");
    var regex = new RegExp("[?&]" + name + "(=([^&#]*)|&|#|$)"),
        results = regex.exec(url);
    if (!results) return null;
    if (!results[2]) return '';
    return decodeURIComponent(results[2].replace(/\+/g, " "));
}