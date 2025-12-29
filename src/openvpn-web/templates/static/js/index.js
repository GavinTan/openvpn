window.vtable = null;
window.gid = null;
window.tables = {};
window.vtable = null;
window.qt = null;
window.now = null;
window.lastMonth = null;

await import('/static/js/utils.js');
await import('/static/js/settings.js');
await import('/static/js/user.js');
await import('/static/js/client.js');
await import('/static/js/cert.js');
await import('/static/js/history.js');

const tooltipTriggerList = document.querySelectorAll('[data-bs-toggle="tooltip"]');
const tooltipList = [...tooltipTriggerList].map((tooltipTriggerEl) => new bootstrap.Tooltip(tooltipTriggerEl));

tables.status = {
  columns: [
    { title: '用户名/客户端', data: 'username' },
    { title: 'VPN IP', data: 'vip' },
    { title: '用户 IP', data: 'rip' },
    { title: '下载流量', data: 'recvBytes' },
    { title: '上传流量', data: 'sendBytes' },
    { title: '上线时间', data: 'connDate' },
    { title: '时长', data: 'onlineTime' },
    {
      title: '操作',
      data: (data) => `<button type="button" class="btn btn-outline-danger btn-sm" id="killClient">断开</button>`,
    },
  ],
  dom:
    "<'d-flex justify-content-between'f<'toolbar'>>" +
    "<'row'<'col-sm-12'tr>>" +
    "<'d-flex justify-content-between align-items-center'lip>",
  fnInitComplete: function () {
    const interval = setInterval(() => {
      if ($('#serverTable').is(':hidden')) {
        clearInterval(interval);
      } else {
        vtable.ajax.reload(null, false);
      }
    }, 30000);
  },
  ajax: function (data, callback, settings) {
    request.get('/ovpn/online-client').then((data) => callback({ data }));
  },
};

const initTable = (tab) => {
  if (tab === 'status') {
    $('#vtableContainer').removeClass('my-3').addClass('my-5');
    $('#serverTable').show();
  } else {
    $('#vtableContainer').removeClass('my-5').addClass('my-3');
    $('#serverTable').hide();
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

$('#manageCert').click(function () {
  window.history.pushState(null, '', '?tab=cert');
  initTable('cert');
});

$('#restartSrv').click(function () {
  $('#restartSrvModal').modal('show');
});

$('#restartSrvSumbit').click(function () {
  request.post('/ovpn/server', { action: 'restartSrv' }).then((data) => {
    $('#restartInfoModal').modal('hide');
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
