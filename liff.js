$(document).ready(function () {
    var liffId = "2000938587-06YmdyJ5";
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
                var formUrl = 'https://docs.google.com/forms/d/e/1FAIpQLSengIKFHfwaRHsqHLe7E776kG8b7jLU6_COZ_7uCUrQIuEd7Q/viewform?embedded=true&usp=pp_url&entry.1916663609='+accessToken;
                var iframe = document.querySelector('iframe');
                iframe.setAttribute('src', formUrl);
            }
        })
        .catch((err) => {
            window.alert('LIFF Initialization failed: ' + err);
        });
}

function showPoint() {
    var apiurl = process.env.API_URL;
    $.getJSON("https://members-api-toslpgfgpq-uc.a.run.app" + '/member', {
        member_id: 'test'
    })
    .done(function(data) {
        console.log(data);
        if (data.results) {
            var result = data.results[0];
            $('#point').val(result.point);
        } else {
            $('#point').val('エラー');
        }
    });
}