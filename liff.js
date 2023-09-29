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
                showPoint(accessToken);
            }
        })
        .catch((err) => {
            window.alert('LIFF Initialization failed: ' + err);
        });
}

function showPoint(token) {
    // var apiurl = "https://members-api-toslpgfgpq-uc.a.run.app";
    var apiurl = "http://localhost:9090";
    $.getJSON(apiurl + '/members', nil)
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
        }
    });
}