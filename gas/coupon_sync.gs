// ============================================================
// 交換特典カタログシート → Firestore reward_catalog 同期
// ============================================================
// Script Properties は serial_generator.gs と共通:
//   FIREBASE_PROJECT_ID : FirebaseのプロジェクトID
//   SERVICE_ACCOUNT_JSON: サービスアカウントのJSONキー（全文）
//
// シート名: "交換特典"
// 列構成（1行目はヘッダー）:
//   A: 定義ID              (例: topping_basic)
//   B: タイトル             (例: 基本トッピングチケット)
//   C: 必要ポイント          (例: 200)
//   D: 説明文               (例: 店舗スタッフにこの画面をご提示ください)
//   E: 有効                 (TRUE / FALSE)
//   F: 画像URL              (例: https://drive.google.com/uc?id=XXXXXXXX)
//   G: 表示順               (例: 1)
//   H: Square Pricing Rule ID (例: 3KKXKOB7LLN5CFOLIXDXFXIZ) ※空欄可
//   I: Square 商品ID        (例: TTXDXT77BJJIAGB5GKJ5MMHC)  ※空欄可・モバイルオーダーURL生成用
// ============================================================

function syncRewardCatalog() {
  var ss = SpreadsheetApp.getActiveSpreadsheet();
  var sheet = ss.getSheetByName('交換特典');
  if (!sheet) {
    SpreadsheetApp.getUi().alert('「交換特典」シートが見つかりません。');
    return;
  }

  var lastRow = sheet.getLastRow();
  if (lastRow < 2) {
    SpreadsheetApp.getUi().alert('データがありません。2行目以降に定義を入力してください。');
    return;
  }

  var data = sheet.getRange(2, 1, lastRow - 1, 9).getValues();
  var token = getAccessToken();
  var writes = [];
  var skipped = 0;

  for (var i = 0; i < data.length; i++) {
    var row = data[i];
    var id             = String(row[0]).trim();
    var title          = String(row[1]).trim();
    var requiredPoints = parseInt(row[2]);
    var description    = String(row[3] || '').trim();
    var active         = row[4] === true || String(row[4]).toUpperCase() === 'TRUE';
    var imageURL       = String(row[5] || '').trim();
    var sortOrder      = parseInt(row[6]) || 0;
    var pricingRuleId  = String(row[7] || '').trim();
    var squareItemId   = String(row[8] || '').trim();

    if (!id || !title || isNaN(requiredPoints) || requiredPoints <= 0) {
      skipped++;
      continue;
    }

    writes.push({
      update: {
        name: FIRESTORE_DOC_PATH + '/reward_catalog/' + id,
        fields: {
          title:           { stringValue: title },
          required_points: { integerValue: requiredPoints },
          description:     { stringValue: description },
          active:          { booleanValue: active },
          image_url:       { stringValue: imageURL },
          sort_order:      { integerValue: sortOrder },
          pricing_rule_id: { stringValue: pricingRuleId },
          square_item_id:  { stringValue: squareItemId }
        }
      }
    });
  }

  if (writes.length === 0) {
    SpreadsheetApp.getUi().alert('書き込むデータがありません。(' + skipped + '行スキップ)');
    return;
  }

  var chunkSize = 500;
  for (var j = 0; j < writes.length; j += chunkSize) {
    var chunk = writes.slice(j, j + chunkSize);
    var response = UrlFetchApp.fetch(FIRESTORE_BASE + ':batchWrite', {
      method: 'POST',
      contentType: 'application/json',
      headers: { Authorization: 'Bearer ' + token },
      payload: JSON.stringify({ writes: chunk }),
      muteHttpExceptions: true
    });
    if (response.getResponseCode() !== 200) {
      throw new Error('Firestore batchWrite 失敗: ' + response.getContentText());
    }
  }

  var msg = writes.length + ' 件を同期しました。';
  if (skipped > 0) msg += '(' + skipped + '行スキップ)';
  SpreadsheetApp.getUi().alert(msg);
  Logger.log(msg);
}

function loadRewardCatalog() {
  var ss = SpreadsheetApp.getActiveSpreadsheet();
  var sheet = ss.getSheetByName('交換特典');
  if (!sheet) {
    SpreadsheetApp.getUi().alert('「交換特典」シートが見つかりません。先に「交換特典シートを作成」を実行してください。');
    return;
  }

  var ui = SpreadsheetApp.getUi();
  var confirm = ui.alert('Firestoreから交換特典カタログを読み込みます。\nシートの内容を上書きしますがよろしいですか？', ui.ButtonSet.OK_CANCEL);
  if (confirm !== ui.Button.OK) return;

  var token = getAccessToken();
  var docs = [];
  var pageToken = null;

  do {
    var url = FIRESTORE_BASE + '/reward_catalog?pageSize=300';
    if (pageToken) url += '&pageToken=' + pageToken;
    var response = UrlFetchApp.fetch(url, {
      method: 'GET',
      headers: { Authorization: 'Bearer ' + token },
      muteHttpExceptions: true
    });
    if (response.getResponseCode() !== 200) {
      throw new Error('Firestore 取得失敗: ' + response.getContentText());
    }
    var parsed = JSON.parse(response.getContentText());
    if (parsed.documents) docs = docs.concat(parsed.documents);
    pageToken = parsed.nextPageToken || null;
  } while (pageToken);

  if (docs.length === 0) {
    ui.alert('Firestoreに交換特典カタログが見つかりませんでした。');
    return;
  }

  var rows = docs.map(function(doc) {
    var f = doc.fields || {};
    var id             = doc.name.split('/').pop();
    var title          = f.title           ? (f.title.stringValue           || '') : '';
    var requiredPoints = f.required_points ? parseInt(f.required_points.integerValue || 0) : 0;
    var description    = f.description     ? (f.description.stringValue     || '') : '';
    var active         = f.active          ? (f.active.booleanValue === true) : false;
    var imageURL       = f.image_url       ? (f.image_url.stringValue       || '') : '';
    var sortOrder      = f.sort_order      ? parseInt(f.sort_order.integerValue || 0) : 0;
    var pricingRuleId  = f.pricing_rule_id ? (f.pricing_rule_id.stringValue  || '') : '';
    var squareItemId   = f.square_item_id  ? (f.square_item_id.stringValue   || '') : '';
    return [id, title, requiredPoints, description, active, imageURL, sortOrder, pricingRuleId, squareItemId];
  });

  rows.sort(function(a, b) { return a[6] - b[6]; });

  var lastRow = sheet.getLastRow();
  if (lastRow > 1) sheet.getRange(2, 1, lastRow - 1, 9).clearContent();

  sheet.getRange(2, 1, rows.length, 9).setValues(rows);

  ui.alert(rows.length + ' 件を読み込みました。');
}

function createRewardCatalogSheet() {
  var ss = SpreadsheetApp.getActiveSpreadsheet();
  var sheet = ss.getSheetByName('交換特典');
  if (sheet) {
    SpreadsheetApp.getUi().alert('「交換特典」シートは既に存在します。');
    return;
  }
  sheet = ss.insertSheet('交換特典');

  var headers = ['定義ID', 'タイトル', '必要ポイント', '説明文', '有効', '画像URL', '表示順', 'Square Pricing Rule ID', 'Square 商品ID'];
  var headerRange = sheet.getRange(1, 1, 1, headers.length);
  headerRange.setValues([headers]);
  headerRange.setFontWeight('bold');
  headerRange.setBackground('#a8d5a2');

  sheet.setFrozenRows(1);
  sheet.setColumnWidth(1, 160);
  sheet.setColumnWidth(2, 220);
  sheet.setColumnWidth(3, 110);
  sheet.setColumnWidth(4, 300);
  sheet.setColumnWidth(5, 80);
  sheet.setColumnWidth(6, 320);
  sheet.setColumnWidth(7, 80);
  sheet.setColumnWidth(8, 240);
  sheet.setColumnWidth(9, 240);

  var sample = [
    ['topping_basic',   '基本トッピングチケット', 200,  '店舗スタッフにこの画面をご提示ください。', true,  '', 1, '', ''],
    ['topping_deluxe',  '豪華トッピングチケット', 400,  '店舗スタッフにこの画面をご提示ください。', true,  '', 2, '', ''],
    ['side_dish',       'おかずチケット',         750,  '店舗スタッフにこの画面をご提示ください。', true,  '', 3, '', ''],
    ['bento',           'お弁当チケット',          1200, '店舗スタッフにこの画面をご提示ください。', true,  '', 4, '', '']
  ];
  sheet.getRange(2, 1, sample.length, 9).setValues(sample);

  SpreadsheetApp.getUi().alert('「交換特典」シートを作成しました。');
}

function onOpen() {
  SpreadsheetApp.getUi()
    .createMenu('シリアル管理')
    .addItem('シリアルナンバー生成', 'generateSerials')
    .addSeparator()
    .addItem('交換特典を読み込み（Firestore → シート）', 'loadRewardCatalog')
    .addItem('交換特典を同期（シート → Firestore）', 'syncRewardCatalog')
    .addItem('交換特典シートを作成', 'createRewardCatalogSheet')
    .addToUi();
}
