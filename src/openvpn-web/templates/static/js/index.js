window.vtable = null;
window.cgid = null;
window.tables = {};
window.qt = null;
window.now = null;
window.lastMonth = null;

await import('/static/js/utils.js');
await import('/static/js/settings.js');
const user = await import('/static/js/user.js');
await import('/static/js/client.js');
await import('/static/js/cert.js');
await import('/static/js/history.js');
await import('/static/js/firewall.js');

const tooltipTriggerList = document.querySelectorAll('[data-bs-toggle="tooltip"]');
const tooltipList = [...tooltipTriggerList].map((tooltipTriggerEl) => new bootstrap.Tooltip(tooltipTriggerEl));

function formatSize(bytes) {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

tables.status = {
  rowId: 'id',
  order: [[5, 'desc']],
  columns: [
    {
      title: '用户名/客户端',
      data: 'username',
      name: 'username',
      className: 'dt-center w-min-160',
      render: (data, type, row) =>
        `<button class="btn btn-link text-decoration-none p-0" id="showOnlineClientOffcanvas">
      ${data == 'UNDEF' ? row.commonName : data}
        </button>`,
    },
    {
      title: 'VPN IP',
      data: 'vip',
      name: 'vip',
      className: 'dt-center w-max-250 text-truncate',
      render: (data, type, row) => `${row.vip && row.vip6 ? `${row.vip}<br />${row.vip6}` : row.vip || row.vip6}`,
    },
    { title: '用户 IP', data: 'rip', name: 'rip', className: 'dt-center w-max-250 text-truncate' },
    // {
    //   title: '上传流量',
    //   data: 'recvBytes',
    //   name: 'recvBytes',
    //   render: (data, type, row) => formatSize(data),
    // },
    // {
    //   title: '下载流量',
    //   data: 'sendBytes',
    //   name: 'sendBytes',
    //   render: (data, type, row) => formatSize(data),
    // },
    {
      title: '上传速率',
      data: 'upSpeed',
      name: 'upSpeed',
      className: 'dt-center w-min-120',
      render: (data, type, row) => {
        if (type === 'sort' || type === 'type') {
          return data || 0;
        }

        return `${formatSize(data || 0)}/s`;
      },
    },
    {
      title: '下载速率',
      data: 'downSpeed',
      name: 'downSpeed',
      className: 'dt-center w-min-120',
      render: (data, type, row) => {
        if (type === 'sort' || type === 'type') {
          return data || 0;
        }

        return `${formatSize(data || 0)}/s`;
      },
    },
    { title: '上线时间', data: 'connDate', name: 'connDate' },
    { title: '时长', data: 'onlineTime', name: 'onlineTime' },
    {
      title: '操作',
      data: null,
      orderable: false,
      searchable: false,
      className: 'dt-center w-min-200',
      render: (data, type, row, meta) => `
      <button type="button" class="btn btn-outline-danger btn-sm" id="killClient">断开</button>
      <button type="button" class="btn btn-outline-primary btn-sm" id="rateLimit">限速</button>
      ${
        data.isNftBlacklist
          ? '<button type="button" class="btn btn-outline-success btn-sm" id="enableNetwork">解网</button>'
          : '<button type="button" class="btn btn-outline-warning btn-sm" id="disableNetwork">禁网</button>'
      }
      `,
    },
  ],
  dom:
    "<'d-md-flex justify-content-between'f<'toolbar'>>" +
    "<'row'<'col-sm-12'tr>>" +
    "<'d-md-flex justify-content-between align-items-center'lip>",
  fnInitComplete: function () {
    const interval = setInterval(() => {
      if ($('#serverTable').is(':hidden')) {
        clearInterval(interval);
      } else {
        vtable.ajax.reload(null, false);
        // vtable.columns.adjust().draw(false);
      }
    }, 1000);
  },
  ajax: function (data, callback, settings) {
    request.get('/ovpn/online-client').then((res) => {
      const now = new Date();
      res?.clients.forEach((i) => {
        i.downSpeed = 0;
        i.upSpeed = 0;

        const row = vtable.row(`#${i.id}`);
        if (row.any()) {
          const old = row.data();
          const t = (now - old.lastTime) / 1000;

          i.downSpeed = (i.sendBytes - old.sendBytes) / t;
          i.upSpeed = (i.recvBytes - old.recvBytes) / t;
        }

        i.lastTime = now;
      });

      $('#serverTable tbody').html(`
        <tr>
          <td>${res?.server.Address}</td>
          <td>${res?.server.Status}</td>
          <td>${res?.server.BytesIn}</td>
          <td>${res?.server.BytesOut}</td>
          <td>${res?.server.RunDate}</td>
        </tr>
      `);

      callback({ data: res?.clients });
    });
  },
};

const initTable = (tab) => {
  $('#treeMenu').empty();
  $('#treeContainer').parent().hide();

  if (tab === 'status') {
    $('#vtableContainer').removeClass('my-3').addClass('my-5');
    $('#serverTable').show();
  } else {
    $('#vtableContainer').removeClass('my-5').addClass('my-3');
    $('#serverTable').hide();
  }

  if (tab === 'user') {
    $('#treeContainer').parent().show();
    user.getTreeData();
  }

  if (tab === 'history') {
    now = new Date();
    lastMonth = new Date(now);
    lastMonth.setMonth(now.getMonth() - 1);
    lastMonth.setHours(0, 0, 0, 0);
    now.setHours(23, 59, 59, 0);
    qt = [Date.parse(lastMonth) / 1000, Date.parse(now) / 1000];
  }

  if (vtable) {
    $('#vtable .btn-delete').popover('dispose');
    vtable.destroy();
    $('#vtable').empty();
  }

  vtable = $('#vtable').DataTable({
    responsive: true,
    colReorder: true,
    language: {
      url: '/static/zh.json',
      loadingRecords: '数据加载中...',
    },
    columnDefs: [{ className: 'dt-center', targets: '_all' }],
    drawCallback: function () {
      $('ul.pagination').addClass('pagination-sm');
    },
    ...tables[tab],
  });
};

const urlParams = new URLSearchParams(window.location.search);
const tabs = Object.keys(tables);

if (tabs.includes(urlParams.get('tab'))) {
  initTable(urlParams.get('tab'));
} else {
  initTable('status');
}

$('#vtable').on('shown.bs.popover', '.btn-delete', function () {
  const popoverInstance = bootstrap.Popover.getInstance(this);
  const popoverEl = $(popoverInstance.tip);
  const row = vtable.row($(this).parents('tr')).data();
  const delType = $(this).data('delete-type');

  if (!popoverInstance) return;

  popoverEl
    .find('.btn-popover-confirm')
    .off('click')
    .on('click', function () {
      switch (delType) {
        case 'user':
          request.delete(`/ovpn/user/${row.id}`).then((data) => {
            popoverInstance.hide();
            message.success(data.message);
            vtable.ajax.reload(null, false);
          });
          break;
        case 'client':
          request.delete(`/ovpn/client/${row.name}`).then((data) => {
            popoverInstance.hide();
            message.success(data.message);
            vtable.ajax.reload(null, false);
          });
          break;
        case 'firewall':
          request.delete(`/ovpn/firewall/${row.id}`).then((data) => {
            popoverInstance.hide();
            message.success(data.message);
            vtable.ajax.reload(null, false);
          });
          break;
      }
    });

  popoverEl
    .find('.btn-popover-cancel')
    .off('click')
    .on('click', function () {
      popoverInstance.hide();
    });
});

$('#vtable').on('show.bs.popover', '.btn-delete', function () {
  $('.btn-delete')
    .not(this)
    .each(function () {
      var popoverInstance = bootstrap.Popover.getInstance(this);
      if (popoverInstance) {
        popoverInstance.hide();
      }
    });
});

$(document).on('click', function (e) {
  if (
    $(e.target).data('toggle') !== 'popover' &&
    $(e.target).parents('.popover.show').length === 0 &&
    $(e.target).parents('.btn-delete').length === 0 &&
    !$(e.target).hasClass('btn-delete')
  ) {
    $('.btn-delete').popover('hide');
  }
});

$('#showUser').click(function () {
  window.history.pushState(null, '', '?tab=user');
  initTable('user');
  if ('{{.ldapAuth}}' == 'true') {
    const toast = $('#alertToast');
    toast.find('.toast-body').text('已启用LDAP认证，本地VPN账号将不在工作！');
    bootstrap.Toast.getOrCreateInstance(toast).show();
  }
});

$('#showHistory').click(function () {
  window.history.pushState(null, '', '?tab=history');
  initTable('history');
});

$('#showClient').click(function () {
  window.history.pushState(null, '', '?tab=client');
  initTable('client');
});

$('#showFirewall').click(function () {
  window.history.pushState(null, '', '?tab=firewall');
  initTable('firewall');
});

$('#manageCert').click(function () {
  window.history.pushState(null, '', '?tab=cert');
  initTable('cert');
});

$('#restartSrv').click(function () {
  $('#restartSrvModal').modal('show');
});

$('#restartSrvSumbit').click(function () {
  request.post('/ovpn/server', { action: 'restartSrv' }).then((data) => {
    $('#restartSrvModal').modal('hide');
    message.success(data.message);
  });
});

$('#sconfig').click(function () {
  request.post('/ovpn/server', { action: 'getConfig' }).then((data) => {
    $('#editServerModal textarea[name="config"]').val(data.content);
    $('#editServerModal').modal('show');
  });
});

$('#editServerSumbit').click(function () {
  const content = $('#editServerModal textarea[name="config"]').val();

  $('#editServerModal').modal('hide');
  request.post('/ovpn/server', { action: 'updateConfig', content }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('click', '#killClient', function () {
  const id = vtable.row($(this).parents('tr')).data().id;

  request.post('/ovpn/kill', { cid: id }).then(() => {
    vtable.cell(this).row().remove().draw();
  });
});

$(document).on('click', '#rateLimit', function () {
  const data = vtable.row($(this).parents('tr')).data();
  $('#rateLimitModal form').trigger('reset');
  request.get(`/ovpn/firewall?a=get_rateLimit&vip=${data.vip || data.vip6}`).then((res) => {
    if (res.upQos.rate) {
      $('#rateLimitModal input[name="upload"]').val(res.upQos.rate);
      $('#rateLimitModal select[name="uploadUnit"]').val(res.upQos.unit);
    }

    if (res.downQos.rate) {
      $('#rateLimitModal input[name="download"]').val(res.downQos.rate);
      $('#rateLimitModal select[name="downloadUnit"]').val(res.downQos.unit);
    }

    $('#rateLimitModal').modal('show');
    $('#rateLimitModal input[name="vip"]').val(
      data.vip && data.vip6 ? `${data.vip},${data.vip6}` : data.vip || data.vip6
    );
  });
});

$('#rateLimitModal form').submit(function (e) {
  e.preventDefault();

  const vip = $('#rateLimitModal input[name="vip"]').val();
  const upload = $('#rateLimitModal input[name="upload"]').val();
  const uploadUnit = $('#rateLimitModal select[name="uploadUnit"]').val();
  const download = $('#rateLimitModal input[name="download"]').val();
  const downloadUnit = $('#rateLimitModal select[name="downloadUnit"]').val();

  request
    .post('/ovpn/firewall?a=set_rateLimit', {
      vip,
      upload,
      uploadUnit,
      download,
      downloadUnit,
    })
    .then((data) => {
      $('#rateLimitModal').modal('hide');
      message.success(data.message);
    });
});

$(document).on('click', '#disableNetwork', function () {
  const data = vtable.row($(this).parents('tr')).data();
  $('#disableNetworkModal h4 span').text(data.vip || data.vip6);
  $('#disableNetworkModal input[name="vip"]').val(
    data.vip && data.vip6 ? `${data.vip},${data.vip6}` : data.vip || data.vip6
  );
  $('#disableNetworkModal').modal('show');
});

$(document).on('click', '#disableNetworkSubmit', function () {
  const vip = $('#disableNetworkModal input[name="vip"]').val();
  request.post('/ovpn/firewall?a=add_blacklist', { vip }).then((data) => {
    $('#disableNetworkModal').modal('hide');
    message.success(data.message);
  });
});

$(document).on('click', '#enableNetwork', function () {
  const data = vtable.row($(this).parents('tr')).data();
  $('#enableNetworkModal h4 span').text(data.vip || data.vip6);
  $('#enableNetworkModal input[name="vip"]').val(
    data.vip && data.vip6 ? `${data.vip},${data.vip6}` : data.vip || data.vip6
  );
  $('#enableNetworkModal').modal('show');
});

$(document).on('click', '#enableNetworkSubmit', function () {
  const vip = $('#enableNetworkModal input[name="vip"]').val();
  request.post('/ovpn/firewall?a=remove_blacklist', { vip }).then((data) => {
    $('#enableNetworkModal').modal('hide');
    message.success(data.message);
  });
});

// 显示在线客户端详情
$(document).on('click', '#showOnlineClientOffcanvas', async function () {
  const data = vtable.row($(this).parents('tr')).data();
  const oc = new bootstrap.Offcanvas($('#onlineClientOffcanvas'));

  const html = `
    <div class="desc-item row">
      <div class="col-5 desc-label">ID</div>
      <div class="col-7 desc-value">${data.id}</div>
    </div>
    <div class="desc-item row">
      <div class="col-5 desc-label">用户名/客户端</div>
      <div class="col-7 desc-value">${data.username}</div>
    </div>
    <div class="desc-item row">
      <div class="col-5 desc-label">用户 IP</div>
      <div class="col-7 desc-value">${data.rip}</div>
    </div>
    <div class="desc-item row">
      <div class="col-5 desc-label">VPN IP</div>
      <div class="col-7 desc-value">${data.vip}</div>
    </div>
   ${
     data.vip6 &&
     `
       <div class="desc-item row">
         <div class="col-5 desc-label">VPN IPV6</div>
         <div class="col-7 desc-value">${data.vip6}</div>
       </div>
     `
   }
    <div class="desc-item row">
      <div class="col-5 desc-label">上传流量</div>
      <div class="col-7 desc-value">${formatSize(data.recvBytes)}</div>
    </div>
    <div class="desc-item row">
      <div class="col-5 desc-label">下载流量</div>
      <div class="col-7 desc-value">${formatSize(data.sendBytes)}</div>
    </div>
    <div class="desc-item row">
      <div class="col-5 desc-label">上线时间</div>
      <div class="col-7 desc-value">${data.connDate}</div>
    </div>
    <div class="desc-item row">
      <div class="col-5 desc-label">时长</div>
      <div class="col-7 desc-value">${data.onlineTime}</div>
    </div>
    <div class="desc-item row">
      <div class="col-5 desc-label">禁止网络</div>
      <div class="col-7 desc-value">${data.isNftBlackList ? '是' : '否'}</div>
    </div>
    `;

  $('#onlineClientOffcanvas .offcanvas-body').html(html);
  oc.show();
});
