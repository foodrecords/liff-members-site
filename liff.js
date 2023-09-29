$(document).ready(function () {
    const liffId = "2000938587-06YmdyJ5";
    initializeLiff(liffId);
    showPoint();
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
                console.log(accessToken);
            }
        })
        .catch((err) => {
            window.alert('LIFF Initialization failed: ' + err);
        });
}

function showPoint() {
    var apiurl = "https://members-api-toslpgfgpq-uc.a.run.app";
    $.getJSON(apiurl + '/members', {
        member_id: 'test'
    })
    .done(function(data) {
        console.log(data);
        if (data.data) {
            $('#point').text(data.data.point + "points");
        } else {
            $('#point').text('エラー');
        }
    });
}