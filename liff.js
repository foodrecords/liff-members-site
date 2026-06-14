var newlyAcquiredIds = {};
var couponDataMap = {};
var rewardDataMap = {};
var currentModalCoupon = null;
var _couponToken = null;
var _currentPoint = 0;
var _currentRank = null;
var _accumulatedResetAt = null;
// secret はフロントエンド非公開。GOLD と同等に扱う。
var RANK_ORDER = { green: 0, bronze: 1, silver: 2, gold: 3, secret: 4 };

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

// ── カスタムダイアログ ─────────────────────────

function showAlert(msg, onClose) {
    var dismiss = function () {
        closeDialog();
        if (onClose) onClose();
    };
    $('#dialog-message').text(msg);
    $('#dialog-cancel-btn').hide();
    $('#dialog-ok-btn').off('click').on('click', dismiss);
    $('#dialog-overlay').off('click').on('click', dismiss);
    $('#custom-dialog').addClass('is-open');
    $('body').addClass('dialog-open');
}

function showConfirm(msg, onOk, onCancel) {
    var dismiss = function () {
        closeDialog();
        if (onCancel) onCancel();
    };
    $('#dialog-message').text(msg);
    $('#dialog-cancel-btn').show().off('click').on('click', dismiss);
    $('#dialog-overlay').off('click').on('click', dismiss);
    $('#dialog-ok-btn').off('click').on('click', function () {
        closeDialog();
        onOk();
    });
    $('#custom-dialog').addClass('is-open');
    $('body').addClass('dialog-open');
}

function closeDialog() {
    $('#custom-dialog').removeClass('is-open');
    $('body').removeClass('dialog-open');
}

function initializeLiff(liffId) {
    liff
        .init({ liffId: liffId })
        .then(() => {
            if (!liff.isInClient() && !liff.isLoggedIn()) {
                showAlert("LINEアカウントでログインするか、LINEアプリから開いてください。", function () {
                    liff.login({ redirectUri: location.href });
                });
            } else {
                const accessToken = liff.getAccessToken();
                if (accessToken) {
                    _couponToken = accessToken;
                    showCoupons(accessToken);
                    showRewards(accessToken);
                    var code = getParam('code');
                    // LIFF によっては ?code=xxx が liff.state に格納される場合があるためフォールバック
                    if (!code) {
                        var liffState = getParam('liff.state');
                        if (liffState) {
                            try { code = getParam('code', decodeURIComponent(liffState)); } catch (e) {}
                        }
                    }
                    // showPoint 完了後に checkCode を実行する。
                    // 並列発火すると GET /members と POST /qrcode が同時に
                    // 新規メンバー判定し、point=0 の GET レスポンスが後から
                    // point=100 の POST 結果を上書きするレースコンディションが発生する。
                    showPoint(accessToken, function () {
                        if (code) checkCode(accessToken, code);
                    });
                }
            }
        })
        .catch((err) => {
            showAlert('LIFF Initialization failed: ' + err);
        });
}

function scanQR() {
    liff
        .scanCodeV2()
        .then((result) => {
            var value = result.value || '';
            // URL に埋め込まれた ?code=xxx を取り出す。
            // QR がそのまま "DL_xxx" 形式の場合も処理できるようにする。
            var code = getParam('code', value);
            if (!code) {
                var m = value.match(/(?:^|[?&])code=(DL_[0-9a-f]+)/i);
                if (!m) m = value.match(/(DL_[0-9a-f]{12})/i);
                if (m) code = m[1];
            }
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

function showPoint(token, onComplete) {
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

                // ポイント失効予定
                if (data.data.next_point_expiry) {
                    var exp = data.data.next_point_expiry;
                    var ed = new Date(exp.expires_at);
                    var edStr = ed.getFullYear() + '/' + (ed.getMonth() + 1) + '/' + ed.getDate();
                    $('#point-card-expiry').text(exp.amount + 'pt が ' + edStr + ' に失効予定').show();
                } else {
                    $('#point-card-expiry').hide();
                }

                _accumulatedResetAt = data.data.accumulated_reset_at || null;
                updateCardRank(data.data.rank, data.data.next_rank, data.data.next_rank_point, data.data.total_earned_point);
                refreshRewardButtons();
                if (data.data.is_new_member) {
                    showWelcomeToast();
                }
            } else {
                $('#point').text('エラー');
            }
            if (onComplete) onComplete();
        },
        error: function (jqXHR) {
            hideLoader();
            var msg = jqXHR.responseJSON && jqXHR.responseJSON.message || jqXHR.statusText || 'network error (status=' + jqXHR.status + ')';
            console.error('[API] /members error:', jqXHR.status, msg);
            showAlert(msg);
            if (onComplete) onComplete();
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
            showAlert(msg);
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
    showConfirm(
        '「' + reward.title + '」と交換しますか？\n' + reward.required_points + ' ポイントを消費します。',
        function () {
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
    );
}

// ── カードランク ──────────────────────────

var RANK_LABELS = { green: 'GREEN', bronze: 'BRONZE', silver: 'SILVER', gold: 'GOLD', secret: 'GOLD' };
var RANK_FLOOR  = { green: 0, bronze: 1000, silver: 3000, gold: 8000, secret: 20000 };

function updateCardRank(rank, nextRank, nextRankPoint, totalEarned) {
    var $card = $('#membership-card');
    $card.removeClass('membership-card--green membership-card--bronze membership-card--silver membership-card--gold');
    // secret は内部ランク。CSS クラスは gold として扱う。
    var cssRank = (rank === 'secret') ? 'gold' : rank;
    if (cssRank) $card.addClass('membership-card--' + cssRank);

    if (_currentRank !== null && rank && (RANK_ORDER[rank] || 0) > (RANK_ORDER[_currentRank] || 0)) {
        showRankUpAnimation(rank);
    }
    _currentRank = rank || _currentRank;

    $('#point-card-next').html(buildRankProgress(rank, nextRank, nextRankPoint, totalEarned || 0, _accumulatedResetAt));
}

function showRankUpAnimation(rank) {
    // secret ランクはフロントエンド非公開のため通知しない
    if (rank === 'secret') return;

    var $card = $('#membership-card');
    $card.removeClass('membership-card--rank-up');
    // force reflow to restart animation
    void $card[0].offsetWidth;
    $card.addClass('membership-card--rank-up');
    setTimeout(function () { $card.removeClass('membership-card--rank-up'); }, 1000);

    var label = RANK_LABELS[rank] || rank.toUpperCase();
    $('#rank-up-toast-text').text('ランクアップ！ ' + label + ' になりました');
    var $toast = $('#rank-up-toast');
    $toast.addClass('is-visible');
    setTimeout(function () { $toast.removeClass('is-visible'); }, 4500);
}

function buildRankProgress(rank, nextRank, nextRankPoint, totalEarned, accumulatedResetAt) {
    var r = 16;
    var circ = +(2 * Math.PI * r).toFixed(2);
    // secret は GOLD ラベルで表示
    var displayRank = (rank === 'secret') ? 'gold' : rank;
    var rankLabel = RANK_LABELS[displayRank] || '';
    var badgeHtml = rankLabel
        ? '<p class="card-rank-badge">' + rankLabel + '</p>'
        : '';

    var resetNoteHtml = '';
    if (accumulatedResetAt) {
        var rd = new Date(accumulatedResetAt);
        var rdStr = rd.getFullYear() + '/' + (rd.getMonth() + 1) + '/' + rd.getDate();
        resetNoteHtml = '<p class="rank-reset-note">累積 ' + rdStr + ' にリセット予定</p>';
    }

    // GOLD（および非公開の secret）はチャートを表示しない
    if (rank === 'gold' || rank === 'secret') {
        return badgeHtml + resetNoteHtml;
    }

    if (!nextRank) {
        return badgeHtml + resetNoteHtml;
    }

    var from = RANK_FLOOR[rank] || 0;
    var current = totalEarned - from;
    var total = current + nextRankPoint;
    var pct = total > 0 ? Math.min(100, Math.max(0, current / total * 100)) : 0;
    var filled = +(circ * pct / 100).toFixed(2);
    var empty   = +(circ - filled).toFixed(2);

    var svgHtml = '<svg class="rank-progress-chart" viewBox="0 0 44 44">' +
        '<circle class="rank-progress-bg" cx="22" cy="22" r="' + r + '"/>' +
        '<circle class="rank-progress-fill" cx="22" cy="22" r="' + r + '"' +
        ' stroke-dasharray="' + filled + ' ' + empty + '"' +
        ' transform="rotate(-90 22 22)"/>' +
        '</svg>';

    var fractionHtml = '<p class="rank-progress-fraction">' +
        current.toLocaleString() +
        '<span>/' + total.toLocaleString() + '</span>' +
        '</p>';

    var nextRankHtml = '<span class="rank-next-name rank-next-name--' + nextRank + '">' +
        RANK_LABELS[nextRank] + '</span>';
    var labelHtml = '<p class="rank-progress-label">あと' + nextRankPoint + 'ptで' + nextRankHtml + '</p>';

    return badgeHtml + svgHtml + fractionHtml + labelHtml + resetNoteHtml;
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
        if (coupon.product_url) {
            $('#modal-mobile-btn').show();
        } else {
            $('#modal-mobile-btn').hide();
        }
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
    showConfirm(
        'スタッフにこの画面をご確認いただいてから「OK」を押してください。\n使用済みにすると元に戻せません。',
        function () {
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
    );
}

function useCouponMobile() {
    if (!currentModalCoupon || !currentModalCoupon.product_url) return;
    var url = currentModalCoupon.product_url;
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
