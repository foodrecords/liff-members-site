var newlyAcquiredIds = {};
var couponDataMap = {};
var currentModalCoupon = null;
var _couponToken = null;

$(document).ready(function () {
    initializeLiff(window.APP_CONFIG.liffId);

    $('#modal-overlay, #modal-close-btn').on('click', closeCouponModal);
    $('#modal-copy-btn').on('click', copyModalCode);
    $('#modal-store-btn').on('click', useCouponInStore);
    $('#modal-mobile-btn').on('click', useCouponMobile);
});

function initializeLiff(liffId) {
    liff
        .init({ liffId: liffId })
        .then(() => {
            if (!liff.isInClient() && !liff.isLoggedIn()) {
                window.alert("LINEアカウントでログインするか、LINEアプリから開いてください。");
                liff.login({ redirectUri: location.href });
            } else {
                const accessToken = liff.getAccessToken();
                if (accessToken) {
                    _couponToken = accessToken;
                    showPoint(accessToken);
                    showCoupons(accessToken);
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
        beforeSend: function (request) {
            request.setRequestHeader('Authorization', 'Bearer ' + token);
        },
        dataType: "json",
        url: apiurl + '/members',
        success: function (data) {
            console.log('[API] /members response:', JSON.stringify(data));
            if (data.data) {
                $('#point-card-balance span').text(data.data.point);
                $('#point-card-number span').text(data.data.number);
                updateCardRank(data.data.rank, data.data.next_rank, data.data.next_rank_point);
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
    _couponToken = token;
    var apiurl = window.APP_CONFIG.apiUrl;
    console.log('[API] POST ' + apiurl + '/qrcode, code=' + code);
    $.ajax({
        beforeSend: function (request) {
            request.setRequestHeader('Authorization', 'Bearer ' + token);
        },
        dataType: "json",
        url: apiurl + '/qrcode',
        type: 'post',
        data: JSON.stringify({ code: code }),
        success: function (data) {
            console.log('[API] /qrcode response:', JSON.stringify(data));
            if (data.data) {
                $('#point-card-balance span').text(data.data.point);
                $('#point-card-number span').text(data.data.number);
                updateCardRank(data.data.rank, data.data.next_rank, data.data.next_rank_point);
                if (data.data.get_point) {
                    $('#point-card-get').text(data.data.get_point + ' point get!').css('visibility', 'visible');
                }
                if (data.data.new_coupons && data.data.new_coupons.length > 0) {
                    data.data.new_coupons.forEach(function (c) {
                        newlyAcquiredIds[c.id] = true;
                    });
                    showCouponToast(data.data.new_coupons);
                }
                showCoupons(token);
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

function showCoupons(token) {
    if (token) _couponToken = token;
    var apiurl = window.APP_CONFIG.apiUrl;
    console.log('[API] GET ' + apiurl + '/coupons');
    $.ajax({
        beforeSend: function (request) {
            request.setRequestHeader('Authorization', 'Bearer ' + _couponToken);
        },
        dataType: "json",
        url: apiurl + '/coupons',
        success: function (data) {
            console.log('[API] /coupons response:', JSON.stringify(data));
            if (data.data) {
                renderCoupons(data.data.coupons);
                renderUpcoming(data.data.upcoming);
            }
        },
        error: function (jqXHR) {
            console.error('[API] /coupons error:', jqXHR.status);
            $('#coupon-list').html('<p class="coupon-empty">クーポンの取得に失敗しました</p>');
        }
    });
}

function renderCoupons(coupons) {
    couponDataMap = {};
    var $list = $('#coupon-list');
    $list.empty();

    if (!coupons || coupons.length === 0) {
        $list.append('<p class="coupon-empty">クーポンはありません</p>');
        return;
    }

    var now = new Date();
    coupons.forEach(function (coupon) {
        couponDataMap[coupon.id] = coupon;

        var expired = coupon.expires_at && new Date(coupon.expires_at) < now;
        var isNew = !!newlyAcquiredIds[coupon.id];

        var classes = 'coupon-card';
        if (coupon.used) classes += ' coupon-card--used';
        else if (expired) classes += ' coupon-card--expired';
        if (isNew) classes += ' coupon-card--new';

        var badgeHtml = coupon.used
            ? '<span class="coupon-badge coupon-badge--used">使用済み</span>'
            : (isNew ? '<span class="coupon-badge coupon-badge--new">NEW</span>' : '');

        var expiryHtml = '';
        if (coupon.expires_at) {
            var d = new Date(coupon.expires_at);
            var dateStr = d.getFullYear() + '/' + (d.getMonth() + 1) + '/' + d.getDate();
            expiryHtml = '<span class="coupon-card-expiry' + (expired ? ' coupon-card-expiry--expired' : '') + '">期限 ' + dateStr + '</span>';
        }

        var html = '<div class="' + classes + '" data-id="' + coupon.id + '">' +
            badgeHtml +
            '<div class="coupon-card-body">' +
            '<p class="coupon-title">' + coupon.title + '</p>' +
            '<div class="coupon-card-meta">' +
            discountHtml(coupon) +
            expiryHtml +
            '</div>' +
            '</div>' +
            '<span class="coupon-card-arrow">&#8250;</span>' +
            '</div>';
        $list.append(html);
    });

    $list.off('click', '.coupon-card').on('click', '.coupon-card', function () {
        var id = $(this).data('id');
        if (couponDataMap[id]) openCouponModal(couponDataMap[id]);
    });
}

// ── カードランク ──────────────────────────

var RANK_LABELS = { green: 'GREEN', bronze: 'BRONZE', silver: 'SILVER', gold: 'GOLD' };

function updateCardRank(rank, nextRank, nextRankPoint) {
    var $card = $('#membership-card');
    $card.removeClass('membership-card--green membership-card--bronze membership-card--silver membership-card--gold');
    if (rank) $card.addClass('membership-card--' + rank);

    $('#point-card-rank').text(RANK_LABELS[rank] || '');

    if (nextRank && nextRankPoint > 0) {
        $('#point-card-next').html((RANK_LABELS[nextRank] || nextRank.toUpperCase()) + 'まで<br>あと ' + nextRankPoint + 'pt');
    } else {
        $('#point-card-next').html('最高ランク<br>達成');
    }
}

// ── モーダル ──────────────────────────────

function openCouponModal(coupon) {
    currentModalCoupon = coupon;

    var now = new Date();
    var expired = coupon.expires_at && new Date(coupon.expires_at) < now;

    $('#modal-title').text(coupon.title);
    $('#modal-code').text(coupon.code);
    $('#modal-copy-btn').text('コピー').removeClass('coupon-copy-btn--copied');

    if (coupon.description) {
        $('#modal-desc').html(nl2br($('<div>').text(coupon.description).html()));
        $('#modal-desc-block').show();
    } else {
        $('#modal-desc-block').hide();
    }

    if (coupon.expires_at) {
        var d = new Date(coupon.expires_at);
        var dateStr = d.getFullYear() + '年' + (d.getMonth() + 1) + '月' + d.getDate() + '日';
        $('#modal-expiry').text((expired ? '（期限切れ）' : '期限: ') + dateStr).show();
    } else {
        $('#modal-expiry').hide();
    }

    if (coupon.used) {
        var usedLabel = '使用済み';
        if (coupon.used_at) {
            var ud = new Date(coupon.used_at);
            usedLabel += '（' + ud.getFullYear() + '/' + (ud.getMonth() + 1) + '/' + ud.getDate() + '）';
        }
        $('#modal-used-note').text(usedLabel).show();
        $('#modal-store-btn').hide();
        $('#modal-mobile-btn').hide();
    } else {
        $('#modal-used-note').hide();
        $('#modal-store-btn').show().prop('disabled', false).text('店舗で使用する');
        $('#modal-mobile-btn').show();
    }

    $('#coupon-modal').addClass('is-open');
    $('body').addClass('modal-open');
}

function closeCouponModal() {
    $('#coupon-modal').removeClass('is-open');
    $('body').removeClass('modal-open');
}

function copyModalCode() {
    if (!currentModalCoupon) return;
    copyToClipboard(currentModalCoupon.code);
    $('#modal-copy-btn').text('コピー済み').addClass('coupon-copy-btn--copied');
    setTimeout(function () {
        $('#modal-copy-btn').text('コピー').removeClass('coupon-copy-btn--copied');
    }, 2000);
}

function useCouponInStore() {
    if (!currentModalCoupon || currentModalCoupon.used) return;
    if (!confirm('スタッフにこの画面をご確認いただいてから「OK」を押してください。\n使用済みにすると元に戻せません。')) return;

    var apiurl = window.APP_CONFIG.apiUrl;
    $('#modal-store-btn').prop('disabled', true).text('処理中...');

    $.ajax({
        beforeSend: function (request) {
            request.setRequestHeader('Authorization', 'Bearer ' + _couponToken);
        },
        dataType: "json",
        url: apiurl + '/coupons/' + currentModalCoupon.id + '/use',
        type: 'post',
        success: function () {
            closeCouponModal();
            showCoupons();
        },
        error: function (jqXHR) {
            var msg = jqXHR.responseJSON && jqXHR.responseJSON.message || 'エラーが発生しました';
            alert(msg);
            $('#modal-store-btn').prop('disabled', false).text('店舗で使用する');
        }
    });
}

function useCouponMobile() {
    if (!currentModalCoupon) return;
    var url = 'https://food-records.square.site/?cc=' + encodeURIComponent(currentModalCoupon.code);
    if (liff.isInClient()) {
        liff.openWindow({ url: url, external: true });
    } else {
        window.open(url, '_blank');
    }
}

// ── トースト ──────────────────────────────

function showCouponToast(newCoupons) {
    var text = newCoupons.length === 1
        ? '「' + newCoupons[0].title + '」を獲得しました！'
        : newCoupons.length + '枚のクーポンを獲得しました！';

    $('#coupon-toast-text').text(text);
    var $toast = $('#coupon-toast');
    $toast.addClass('is-visible');

    setTimeout(function () {
        $toast.removeClass('is-visible');
    }, 3200);

    // クーポンセクションへスクロール
    setTimeout(function () {
        var $section = $('.coupons-section');
        if ($section.length) {
            $section[0].scrollIntoView({ behavior: 'smooth' });
        }
    }, 600);
}

// ── 次回の特典 ────────────────────────────

function renderUpcoming(upcoming) {
    var $section = $('#upcoming-section');
    var $list = $('#upcoming-list');
    $list.empty();

    if (!upcoming || upcoming.length === 0) {
        $section.hide();
        return;
    }

    $section.show();

    // 最初の非ランクゴール → NEXT として表示（なければ何も表示しない）
    var firstNonGoal = null;
    for (var i = 0; i < upcoming.length; i++) {
        if (!upcoming[i].is_rank_goal) { firstNonGoal = upcoming[i]; break; }
    }
    if (firstNonGoal) {
        $list.append(buildUpcomingCard(firstNonGoal, 'NEXT'));
    }

    // 未取得のランクゴールをすべて表示
    upcoming.forEach(function (item) {
        if (item.is_rank_goal) {
            $list.append(buildUpcomingCard(item, 'RANK GOAL'));
        }
    });
}

function buildUpcomingCard(item, label) {
    var isGoal = label === 'RANK GOAL';
    var discountStr = item.discount_label
        || (item.discount_amount > 0 ? '¥' + item.discount_amount.toLocaleString() + ' OFF' : '');
    return '<div class="upcoming-card' + (isGoal ? ' upcoming-card--goal' : '') + '">' +
        '<div class="upcoming-card-label">' + label + '</div>' +
        '<div class="upcoming-card-top">' +
        '<p class="upcoming-title">' + item.title + '</p>' +
        (discountStr ? '<span class="upcoming-discount">' + discountStr + '</span>' : '') +
        '</div>' +
        '<div class="upcoming-meta">' +
        '<span class="upcoming-milestone">' + item.point_milestone + 'pt 達成で獲得</span>' +
        '<span class="upcoming-points-needed">あと <strong>' + item.points_needed + '</strong> pt</span>' +
        '</div>' +
        '<div class="upcoming-progress-track"><div class="upcoming-progress-fill" style="width:' + item.progress_percent + '%"></div></div>' +
        '</div>';
}

// ── ユーティリティ ────────────────────────

function discountHtml(coupon) {
    var label = coupon.discount_label
        || (coupon.discount_amount > 0 ? '¥' + coupon.discount_amount.toLocaleString() + ' OFF' : '');
    return label ? '<span class="coupon-card-discount">' + label + '</span>' : '';
}

function copyToClipboard(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).catch(function () {
            fallbackCopy(text);
        });
    } else {
        fallbackCopy(text);
    }
}

function fallbackCopy(text) {
    var $tmp = $('<textarea>').val(text).appendTo('body');
    $tmp[0].select();
    document.execCommand('copy');
    $tmp.remove();
}

function nl2br(str) {
    return str.replace(/\n/g, '<br>');
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
