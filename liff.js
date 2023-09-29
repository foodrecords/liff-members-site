$(document).ready(function () {
    const liffId = "2000938587-06YmdyJ5";
    initializeLiff(liffId);
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
    var apiurl = "https://members-api-toslpgfgpq-uc.a.run.app";
    // var apiurl = "http://localhost:9090";
    $.ajax({
        beforeSend: function(request) {
            request.setRequestHeader('Authorization', 'Bearer '+token);
        },
        dataType: "json",
        url: apiurl + '/members',
        success: function(data) {
            console.log(data);
            if (data.data) {
                $('#point-card-balance span').text(data.data.point);
                $('#point-card-number span').text(data.data.number);
            } else {
                $('#point').text('エラー');
            }
        },
        error: function (jqXHR) {
            alert(jqXHR.responseJSON.message);
        }
    });
}

function checkCode(token, code) {    
    var apiurl = "https://members-api-toslpgfgpq-uc.a.run.app";
    // var apiurl = "http://localhost:9090";
    $.ajax({
        beforeSend: function(request) {
            request.setRequestHeader('Authorization', 'Bearer '+token);
        },
        dataType: "json",
        url: apiurl + '/qrcode',
        type: 'post',
        data: JSON.stringify({ code: code }),
        success: function(data) {
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
            alert(jqXHR.responseJSON.message);
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