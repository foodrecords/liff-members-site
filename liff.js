var newlyAcquiredIds = {};
var couponDataMap = {};
var rewardDataMap = {};
var currentModalCoupon = null;
var _couponToken = null;
var _currentPoint = 0;

$(document).ready(function () {
    initializeLiff(window.APP_CONFIG.liffId);

    $('#modal-overlay, #modal-close-btn').on('click', closeCouponModal);
    $('#modal-store-btn').on('click', useCouponInStore);
    $('#modal-mobile-btn').on('click', useCouponMobile);
    $('#modal-reward-exchange-btn').on('click', function () {
        var id = $(this).data('reward-id');
        if (rewardDataMap[id]) {
            closeCouponModal();
            confirmExchange(rewardDataMap[id]);
        }
    });
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
                    showRewards(accessToken);
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
    liff
        .scanCodeV2()
        .then((result) => {
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

function hideLoader() {
    var $overlay = $('#loading-overlay');
    $overlay.addClass('is-hidden');
    setTimeout(function () { $overlay.remove(); }, 450);
}

function showPoint(token) {
    var apiurl = window.APP_CONFIG.apiUrl;
    $.ajax({
        beforeSend: function (request) {
            request.setRequestHeader('Authorization', 'Bearer ' + token);
        },
        dataType: "json",
        url: apiurl + '/members',
        success: function (data) {
            hideLoader();
            if (data.data) {
                _currentPoint = data.data.point;
                $('#point-card-balance span').text(data.data.point);
                $('#point-card-number span').text(data.data.number);
                $('#point-card-name').text(data.data.name || '');
                updateCardRank(data.data.rank, data.data.next_rank, data.data.next_rank_point, data.data.total_earned_point);
                if (data.data.is_new_member) {
                    showWelcomeToast();
                }
            } else {
                $('#point').text('エラー');
            }
        },
        error: function (jqXHR) {
            hideLoader();
            var msg = jqXHR.responseJSON && jqXHR.responseJSON.message || jqXHR.statusText || 'network error (status=' + jqXHR.status + ')';
            console.error('[API] /members error:', jqXHR.status, msg);
            alert(msg);
        }
    });
}

function checkCode(token, code) {
    _couponToken = token;
    var apiurl = window.APP_CONFIG.apiUrl;
    $.ajax({
        beforeSend: function (request) {
            request.setRequestHeader('Authorization', 'Bearer ' + token);
        },
        dataType: "json",
        url: apiurl + '/qrcode',
        type: 'post',
        data: JSON.stringify({ code: code }),
        success: function (data) {
            if (data.data) {
                _currentPoint = data.data.point;
                $('#point-card-balance span').text(data.data.point);
                $('#point-card-number span').text(data.data.number);
                updateCardRank(data.data.rank, data.data.next_rank, data.data.next_rank_point, data.data.total_earned_point);
                if (data.data.get_point) {
                    $('#point-card-get').text(data.data.get_point + ' point get!').css('visibility', 'visible');
                    showPointToast(data.data.get_point);
                }
                var cleanUrl = new URL(window.location.href);
                cleanUrl.searchParams.delete('code');
                history.replaceState(null, '', cleanUrl.toString());
                showCoupons(token);
                refreshRewardButtons();
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
    $.ajax({
        beforeSend: function (request) {
            request.setRequestHeader('Authorization', 'Bearer ' + _couponToken);
        },
        dataType: "json",
        url: apiurl + '/coupons',
        success: function (data) {
            if (data.data) {
                renderCoupons(data.data.coupons);
            }
        },
        error: function (jqXHR) {
            console.error('[API] /coupons error:', jqXHR.status);
            $('#coupon-list').html('<p class="coupon-empty">クーポンの取得に失敗しました</p>');
        }
    });
}

function showRewards(token) {
    if (token) _couponToken = token;
    var apiurl = window.APP_CONFIG.apiUrl;
    $.ajax({
        beforeSend: function (request) {
            request.setRequestHeader('Authorization', 'Bearer ' + _couponToken);
        },
        dataType: "json",
        url: apiurl + '/rewards',
        success: function (data) {
            if (data.data) {
                renderRewards(data.data);
            }
        },
        error: function (jqXHR) {
            console.error('[API] /rewards error:', jqXHR.status);
            $('#reward-list').html('<p class="coupon-empty">特典の取得に失敗しました</p>');
        }
    });
}

// ── 獲得済みの特典 ──────────────────────────

function renderCoupons(coupons) {
    couponDataMap = {};
    var $list = $('#coupon-list');
    $list.empty();

    if (!coupons || coupons.length === 0) {
        $list.append('<p class="coupon-empty">獲得済みの特典はありません</p>');
        return;
    }

    var now = new Date();

    var sorted = coupons.slice().sort(function (a, b) {
        var rank = function (c) {
            if (c.used) return 2;
            if (c.expires_at && new Date(c.expires_at) < now) return 1;
            return 0;
        };
        return rank(a) - rank(b);
    });

    sorted.forEach(function (coupon) {
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

        var thumbHtml = coupon.image_url
            ? '<div class="coupon-card-thumb"><img src="' + coupon.image_url + '" alt=""></div>'
            : '';

        var metaHtml = discountHtml(coupon) + expiryHtml;

        var html = '<div class="' + classes + '" data-id="' + coupon.id + '">' +
            badgeHtml +
            thumbHtml +
            '<div class="coupon-card-body">' +
            '<p class="coupon-title">' + coupon.title + '</p>' +
            '<div class="coupon-card-meta">' + metaHtml + '</div>' +
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

// ── ポイントを使う ────────────────────────────

function renderRewards(rewards) {
    rewardDataMap = {};
    var $list = $('#reward-list');
    $list.empty();

    if (!rewards || rewards.length === 0) {
        $list.append('<p class="coupon-empty">交換できる特典はありません</p>');
        return;
    }

    rewards.forEach(function (reward) {
        rewardDataMap[reward.id] = reward;
        $list.append(buildRewardCard(reward));
    });

    $list.off('click', '.reward-card').on('click', '.reward-card', function () {
        var id = $(this).data('id');
        if (rewardDataMap[id]) openRewardModal(rewardDataMap[id]);
    });

    $list.off('click', '.reward-exchange-btn').on('click', '.reward-exchange-btn', function (e) {
        e.stopPropagation();
        var id = $(this).closest('.reward-card').data('id');
        if (rewardDataMap[id]) confirmExchange(rewardDataMap[id]);
    });
}

function buildRewardCard(reward) {
    var canAfford = _currentPoint >= reward.required_points;
    var thumbHtml = reward.image_url
        ? '<div class="reward-card-thumb"><img src="' + reward.image_url + '" alt=""></div>'
        : '';
    var btnClass = 'reward-exchange-btn' + (canAfford ? '' : ' reward-exchange-btn--disabled');
    var btnText = canAfford ? '交換する' : '残高不足';

    return '<div class="reward-card" data-id="' + reward.id + '">' +
        thumbHtml +
        '<div class="reward-card-body">' +
        '<p class="reward-card-title">' + reward.title + '</p>' +
        '<p class="reward-card-cost"><span class="reward-card-cost-value">' + reward.required_points + '</span> pt</p>' +
        '</div>' +
        '<button class="' + btnClass + '" ' + (canAfford ? '' : 'disabled') + '>' + btnText + '</button>' +
        '</div>';
}

function refreshRewardButtons() {
    $('#reward-list .reward-card').each(function () {
        var id = $(this).data('id');
        var reward = rewardDataMap[id];
        if (!reward) return;
        var canAfford = _currentPoint >= reward.required_points;
        var $btn = $(this).find('.reward-exchange-btn');
        $btn.toggleClass('reward-exchange-btn--disabled', !canAfford)
            .prop('disabled', !canAfford)
            .text(canAfford ? '交換する' : '残高不足');
    });
}

function confirmExchange(reward) {
    if (!confirm('「' + reward.title + '」と交換しますか？\n' + reward.required_points + ' ポイントを消費します。')) return;

    var apiurl = window.APP_CONFIG.apiUrl;
    $.ajax({
        beforeSend: function (request) {
            request.setRequestHeader('Authorization', 'Bearer ' + _couponToken);
        },
        dataType: "json",
        url: apiurl + '/rewards/' + reward.id + '/exchange',
        type: 'post',
        success: function (data) {
            if (data.data) {
                _currentPoint = data.data.new_point;
                $('#point-card-balance span').text(_currentPoint);
                newlyAcquiredIds[data.data.coupon_id] = true;
                showCouponToast(data.data.title);
                showCoupons();
                refreshRewardButtons();
            }
        },
        error: function (jqXHR) {
            var msg = jqXHR.responseJSON && jqXHR.responseJSON.message || 'エラーが発生しました';
            showErrorBanner(msg);
        }
    });
}

// ── カードランク ──────────────────────────

var RANK_LABELS = { green: 'GREEN', bronze: 'BRONZE', silver: 'SILVER', gold: 'GOLD' };
var RANK_FLOOR  = { green: 0, bronze: 1000, silver: 3000, gold: 8000 };

function updateCardRank(rank, nextRank, nextRankPoint, totalEarned) {
    var $card = $('#membership-card');
    $card.removeClass('membership-card--green membership-card--bronze membership-card--silver membership-card--gold');
    if (rank) $card.addClass('membership-card--' + rank);

    $('#point-card-rank').text(RANK_LABELS[rank] || '');
    $('#point-card-next').html(buildRankChart(rank, nextRank, nextRankPoint, totalEarned || 0));
}

function buildRankChart(rank, nextRank, nextRankPoint, totalEarned) {
    var r = 16;
    var circ = +(2 * Math.PI * r).toFixed(2);

    var filled, empty, innerHtml;
    if (!nextRank) {
        filled = circ;
        empty = 0;
        innerHtml = '<text class="rank-chart-value" x="22" y="26" text-anchor="middle">MAX</text>';
    } else {
        var from = RANK_FLOOR[rank] || 0;
        var span = (totalEarned + nextRankPoint) - from;
        var pct = span > 0 ? Math.min(100, Math.max(0, (totalEarned - from) / span * 100)) : 0;
        filled = +(circ * pct / 100).toFixed(2);
        empty  = +(circ - filled).toFixed(2);
        var valueFontSize = String(nextRankPoint).length >= 4 ? '8.5px' : '11px';
        var valueY = String(nextRankPoint).length >= 4 ? 32 : 31;
        innerHtml =
            '<text class="rank-chart-label" x="22" y="20" text-anchor="middle">あと</text>' +
            '<text class="rank-chart-value" style="font-size:' + valueFontSize + '" x="22" y="' + valueY + '" text-anchor="middle">' + nextRankPoint + 'pt</text>';
    }

    return '<svg class="rank-progress-chart" viewBox="0 0 44 44">' +
        '<circle class="rank-progress-bg" cx="22" cy="22" r="' + r + '"/>' +
        '<circle class="rank-progress-fill" cx="22" cy="22" r="' + r + '"' +
        ' stroke-dasharray="' + filled + ' ' + empty + '"' +
        ' transform="rotate(-90 22 22)"/>' +
        innerHtml +
        '</svg>';
}

// ── モーダル ──────────────────────────────

function openCouponModal(coupon) {
    currentModalCoupon = coupon;

    var now = new Date();
    var expired = coupon.expires_at && new Date(coupon.expires_at) < now;

    $('#modal-title').text(coupon.title);

    if (coupon.image_url) {
        $('#modal-image').attr('src', coupon.image_url).show();
    } else {
        $('#modal-image').attr('src', '').hide();
    }

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

    $('#modal-reward-exchange-btn').hide();

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

function openRewardModal(reward) {
    var canAfford = _currentPoint >= reward.required_points;

    $('#modal-title').text(reward.title);

    if (reward.image_url) {
        $('#modal-image').attr('src', reward.image_url).show();
    } else {
        $('#modal-image').attr('src', '').hide();
    }

    if (reward.description) {
        $('#modal-desc').html(nl2br($('<div>').text(reward.description).html()));
        $('#modal-desc-block').show();
    } else {
        $('#modal-desc-block').hide();
    }

    $('#modal-expiry').hide();
    $('#modal-used-note').hide();
    $('#modal-store-btn').hide();
    $('#modal-mobile-btn').hide();

    var btnText = canAfford
        ? reward.required_points + ' pt で交換する'
        : '残高不足（' + reward.required_points + ' pt 必要）';
    $('#modal-reward-exchange-btn')
        .text(btnText)
        .toggleClass('coupon-modal-exchange-btn--disabled', !canAfford)
        .prop('disabled', !canAfford)
        .data('reward-id', reward.id)
        .show();

    $('#coupon-modal').addClass('is-open');
    $('body').addClass('modal-open');
}

function closeCouponModal() {
    $('#coupon-modal').removeClass('is-open');
    $('body').removeClass('modal-open');
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
            showErrorBanner(msg);
            $('#modal-store-btn').prop('disabled', false).text('店舗で使用する');
        }
    });
}

function useCouponMobile() {
    if (!currentModalCoupon) return;
    var url = currentModalCoupon.product_url || 'https://food-records.square.site/';
    if (liff.isInClient()) {
        liff.openWindow({ url: url, external: false });
    } else {
        window.open(url, '_blank');
    }
}

// ── トースト ──────────────────────────────

function showErrorBanner(msg) {
    $('#error-toast-text').text(msg);
    var $toast = $('#error-toast');
    $toast.addClass('is-visible');
    setTimeout(function () {
        $toast.removeClass('is-visible');
    }, 3500);
}

function showPointToast(point) {
    $('#point-toast-text').text('+' + point + ' ポイント獲得！');
    var $toast = $('#point-toast');
    $toast.addClass('is-visible');
    setTimeout(function () {
        $toast.removeClass('is-visible');
    }, 2400);
}

function showWelcomeToast() {
    var $toast = $('#welcome-toast');
    $toast.addClass('is-visible');
    setTimeout(function () {
        $toast.removeClass('is-visible');
    }, 4200);
}

function showCouponToast(title) {
    var text = '「' + title + '」を獲得しました！';
    $('#coupon-toast-text').text(text);
    var $toast = $('#coupon-toast');
    $toast.addClass('is-visible');

    setTimeout(function () {
        $toast.removeClass('is-visible');
    }, 3200);

    setTimeout(function () {
        switchTab('acquired');
        var $tabs = $('#coupon-tabs');
        if ($tabs.length) {
            $tabs[0].scrollIntoView({ behavior: 'smooth' });
        }
    }, 600);
}

// ── タブ ─────────────────────────────────

function switchTab(tab) {
    if (tab === 'acquired') {
        $('#tab-acquired').addClass('is-active');
        $('#tab-exchange').removeClass('is-active');
        $('#tab-panel-acquired').show();
        $('#tab-panel-exchange').hide();
    } else {
        $('#tab-exchange').addClass('is-active');
        $('#tab-acquired').removeClass('is-active');
        $('#tab-panel-exchange').show();
        $('#tab-panel-acquired').hide();
    }
}

// ── ユーティリティ ────────────────────────

function discountHtml(coupon) {
    var label = coupon.discount_label
        || (coupon.discount_amount > 0 ? '¥' + coupon.discount_amount.toLocaleString() + ' OFF' : '');
    return label ? '<span class="coupon-card-discount">' + label + '</span>' : '';
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
