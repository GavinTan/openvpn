tables.client = {
  columns: [
    { title: '名称', data: 'name' },
    { title: '日期', data: 'date' },
    {
      title: '操作',
      data: (data) => {
        const html = `
        <div class="d-grid gap-2 d-flex justify-content-center align-items-center">
          <a href="${data.file}" download="${data.fullName}" class="text-decoration-none">下载</a>
          <div class="dropdown">
            <button class="btn btn-link text-decoration-none p-0 dropdown-toggle" type="button" data-bs-toggle="dropdown" aria-expanded="false">
              编辑
            </button>
            <ul class="dropdown-menu">
              <li><button type="button" class="dropdown-item" id="editCCD">CCD配置</button></li>
              <li><button type="button" class="dropdown-item" id="editClient">客户端配置</button></li>
            </ul>
          </div>
          <button class="btn btn-link text-decoration-none p-0 btn-delete" data-bs-toggle="popover" data-delete-type="client" data-delete-name="${data.name}">删除</button>
        </div>
        `;
        return html;
      },
    },
  ],
  order: [[1, 'desc']],
  buttons: {
    dom: {
      button: { className: 'btn btn-sm' },
    },
    buttons: [
      {
        text: '添加',
        className: 'btn-primary',
        action: () => $('#addClientModal').modal('show'),
      },
    ],
  },
  dom:
    "<'d-flex justify-content-between align-items-center'fB>" +
    "<'row'<'col-sm-12'tr>>" +
    "<'d-flex justify-content-between align-items-center'lip>",
  drawCallback: function (settings) {
    $('#vtable .btn-delete').popover('dispose');
    $('#vtable .btn-delete').popover({
      container: 'body',
      placement: 'top',
      html: true,
      sanitize: false,
      trigger: 'click',
      title: '提示',
      content: function () {
        const name = $(this).data('delete-name');
        return `
          <div>
            <p>确定删除 <strong>${name}</strong> 吗？</p>
            <div class="d-flex justify-content-center">
              <button class="btn btn-secondary btn-sm me-2 btn-popover-cancel">取消</button>
              <button class="btn btn-primary btn-sm btn-popover-confirm">确认</button>
            </div>
          </div>
        `;
      },
    });
  },
  ajax: function (data, callback, settings) {
    request.get('/ovpn/client').then((data) => callback({ data }));
  },
};

// 添加客户端
$('#addClientModal form').submit(function () {
  const name = $('#addClientModal input[name="name"]').val();
  const serverAddr = $('#addClientModal input[name="serverAddr"]').val() || location.hostname;
  const serverPort = $('#addClientModal input[name="serverPort"]').val();
  const config = $('#addClientModal textarea[name="config"]').val();
  const ccdConfig = $('#addClientModal textarea[name="ccdConfig"]').val();
  const mfa = $('#addClientModal input[name="mfa"]').is(':checked');

  request.post('/ovpn/client', { name, serverAddr, serverPort, config, ccdConfig, mfa }).then((data) => {
    vtable.ajax.reload(null, false);
    $('#addClientModal').modal('hide');
    $('#addClientModal form').trigger('reset');
    message.success(data.message);
  });

  return false;
});

// 编辑客户端
$(document).on('click', '#editClient', function () {
  const name = vtable.row($(this).parents('tr')).data().name;
  $('#editClientModal input[name="name"]').val(`clients/${name}.ovpn`);

  request.get(`/ovpn/client?a=getConfig&file=clients/${encodeURIComponent(name)}.ovpn`).then((data) => {
    $('#editClientModal textarea[name="config"]').val(data.content);
    $('#editClientModal').modal('show');
  });
});

$('#editClientSumbit').click(function () {
  const name = $('#editClientModal input[name="name"]').val();
  const content = $('#editClientModal textarea[name="config"]').val();

  $('#editClientModal').modal('hide');
  request.put(`/ovpn/client?file=${encodeURIComponent(name)}`, { content }).then((data) => {
    message.success(data.message);
  });
});

// 编辑CCD
$(document).on('click', '#editCCD', function () {
  const name = vtable.row($(this).parents('tr')).data().name;
  $('#editClientModal input[name="name"]').val(`ccd/${name}`);

  request.get(`/ovpn/client?a=getConfig&file=ccd/${encodeURIComponent(name)}`).then((data) => {
    $('#editClientModal textarea[name="config"]').val(data.content);
    $('#editClientModal').modal('show');
  });
});
