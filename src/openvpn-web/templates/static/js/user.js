tables.user = {
  autoWidth: false,
  responsive: true,
  columns: [
    {
      title: 'ID',
      data: 'id',
      visible: false,
      searchable: false,
    },
    {
      title: '用户名',
      data: (data) =>
        `<button class="btn btn-link text-decoration-none p-0" id="showUserOffcanvas">${data.username}</button>`,
    },
    {
      title: '密码',
      data: (data) => {
        if (data.password.length == 0) {
          return data.password;
        }

        const html = `
        <div class="form-group d-flex justify-content-center align-items-center gap-1">
          <input class="border border-0 p-0 bg-transparent" style="outline: none;width: ${
            data.password.length * 7
          }px;max-width: 175px;" value="${data.password}" type="password" autocomplete="current-password" readonly>
          <button class="btn btn-link p-0" id="copyPass">
            <svg viewBox="64 64 896 896" focusable="false" data-icon="copy" width="1em" height="1em" fill="currentColor" aria-hidden="true">
              <path d="M832 64H296c-4.4 0-8 3.6-8 8v56c0 4.4 3.6 8 8 8h496v688c0 4.4 3.6 8 8 8h56c4.4 0 8-3.6 8-8V96c0-17.7-14.3-32-32-32zM704 192H192c-17.7 0-32 14.3-32 32v530.7c0 8.5 3.4 16.6 9.4 22.6l173.3 173.3c2.2 2.2 4.7 4 7.4 5.5v1.9h4.2c3.5 1.3 7.2 2 11 2H704c17.7 0 32-14.3 32-32V224c0-17.7-14.3-32-32-32zM350 856.2L263.9 770H350v86.2zM664 888H414V746c0-22.1-17.9-40-40-40H232V264h432v624z"></path>
            </svg>
          </button>
        </div>
        `;
        return html;
      },
    },
    { title: 'IP地址', data: 'ipAddr' },
    { title: '配置文件', data: 'ovpnConfig' },
    {
      title: 'MFA',
      data: (data) => (data.mfaSecret ? '开启' : ''),
    },
    {
      title: '状态',
      data: (data) => {
        const ed = new Date(data.expireDate);
        const now = new Date();
        ed.setHours(0, 0, 0, 0);
        now.setHours(0, 0, 0, 0);
        if (ed < now) {
          return `<span class="badge text-bg-danger">已过期</span>`;
        }

        return data.isEnable
          ? `<span class="badge text-bg-success">启用</span>`
          : `<span class="badge text-bg-secondary">禁用</span>`;
      },
    },
    { title: '姓名', data: 'name' },
    {
      title: '操作',
      data: (data) => {
        const html = `
        <button class="btn btn-link text-decoration-none p-0" id="editUser">编辑</button>
        ${
          data.isEnable === true
            ? '<button class="btn btn-link text-decoration-none p-0" id="disableUser">禁用</button>'
            : '<button class="btn btn-link text-decoration-none p-0" id="enableUser">启用</button>'
        }
        <button class="btn btn-link text-decoration-none p-0 btn-delete" data-bs-toggle="popover" data-delete-type="user" data-delete-name="${
          data.username
        }">删除</button>
        <div class="btn btn-link text-decoration-none p-0 dropdown">
          <button class="btn btn-link text-decoration-none p-0 dropdown-toggle" type="button" data-bs-toggle="dropdown" aria-expanded="false">
            更多
          </button>
          <ul class="dropdown-menu">
            <li><a class="dropdown-item" id="resetPass">重置密码</a></li>
            <li><a class="dropdown-item" id="resetMfa">重置MFA</a></li>
          </ul>
        </div>
        `;
        return html;
      },
    },
  ],
  order: [[0, 'desc']],
  buttons: {
    dom: {
      button: { className: 'btn btn-sm' },
    },
    buttons: [
      {
        text: '导入',
        className: 'btn-primary border-end',
        action: () => {
          $('#importUserModal').modal('show');
        },
      },
      {
        text: '添加',
        className: 'btn-primary border-start',
        action: () => {
          const elem = document.querySelector('#addUserModal input[name="expireDate"]');
          const datepicker = new Datepicker(elem, {
            buttonClass: 'btn',
            format: 'yyyy-mm-dd',
            autohide: true,
            language: 'zh-CN',
            orientation: 'top',
            minDate: new Date(),
          });

          request.get('/ovpn/client').then((data) => {
            $('#addUserModal select[name="ovpnConfig"]').html(
              data.map((i) => `<option value="${i.fullName}">${i.name}</option>`)
            );

            $('#addUserModal').modal('show');
          });
        },
      },
    ],
  },
  dom:
    "<'row align-items-center'<'col d-flex'f><'col d-flex justify-content-center toolbar'><'col d-flex justify-content-end'B>>" +
    "<'row'<'col-sm-12'tr>>" +
    "<'d-flex justify-content-between align-items-center'lip>",
  fnInitComplete: function (oSettings, data) {
    $('#vtable_wrapper div.toolbar').html(
      `<div class="form-check form-switch form-check-reverse">
        <input class="form-check-input" type="checkbox" role="switch" id="authUser" style="cursor: pointer;" ${
          data.authUser ? 'checked' : ''
        }>
        <label class="form-check-label">账号启用: </label>
      </div>
      `
    );
  },
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
    request.get(`/ovpn/user/${cgid}`).then((data) => callback({ data: data?.users, authUser: data?.authUser }));
  },
};

// 显示用户详情
$(document).on('click', '#showUserOffcanvas', async function () {
  const data = vtable.row($(this).parents('tr')).data();
  const oc = new bootstrap.Offcanvas($('#userOffcanvas'));

  const ed = new Date(data.expireDate);
  const now = new Date();
  ed.setHours(0, 0, 0, 0);
  now.setHours(0, 0, 0, 0);

  const group = await request.get(`/ovpn/group/${data.gid}`);

  const html = `
    <div class="desc-item row">
      <div class="col-5 desc-label">ID</div>
      <div class="col-7 desc-value">${data.id}</div>
    </div>
    <div class="desc-item row">
      <div class="col-5 desc-label">用户名</div>
      <div class="col-7 desc-value">${data.username}</div>
    </div>
    <div class="desc-item row">
      <div class="col-5 desc-label">密码</div>
      <div class="col-7 desc-value">${data.password}</div>
    </div>
    <div class="desc-item row">
      <div class="col-5 desc-label">IP地址</div>
      <div class="col-7 desc-value">${data.ipAddr}</div>
    </div>
    <div class="desc-item row">
      <div class="col-5 desc-label">配置文件</div>
      <div class="col-7 desc-value">${data.ovpnConfig}</div>
    </div>
    <div class="desc-item row">
      <div class="col-5 desc-label">MFA</div>
      <div class="col-7 desc-value">${data.mfaSecret}</div>
    </div>
    <div class="desc-item row">
      <div class="col-5 desc-label">状态</div>
      <div class="col-7 desc-value">${ed < now ? '已过期' : data.isEnable ? '启用' : '禁用'}</div>
    </div>
    <div class="desc-item row">
      <div class="col-5 desc-label">姓名</div>
      <div class="col-7 desc-value">${data.name}</div>
    </div>
      <div class="desc-item row">
      <div class="col-5 desc-label">节点</div>
      <div class="col-7 desc-value">${group?.name || ''}</div>
    </div>
    <div class="desc-item row">
      <div class="col-5 desc-label">过期时间</div>
      <div class="col-7 desc-value">
        ${data.expireDate ? dayjs(data.expireDate).format('YYYY-MM-DD HH:mm:ss') : ''}
      </div>
    </div>
    <div class="desc-item row">
      <div class="col-5 desc-label">创建时间</div>
      <div class="col-7 desc-value">${dayjs(data.createdAt).format('YYYY-MM-DD HH:mm:ss')}</div>
    </div>
    <div class="desc-item row">
      <div class="col-5 desc-label">更新时间</div>
      <div class="col-7 desc-value">${dayjs(data.updatedAt).format('YYYY-MM-DD HH:mm:ss')}</div>
    </div>
    `;

  $('#userOffcanvas .offcanvas-body').html(html);
  oc.show();
});

// 导入用户
let files = [];
const uploadFile = new FormData();
const importUserFileDropZone = document.querySelector('#importUserModal .file-drop-zone');
const importUserFileInput = document.querySelector('#importUserModal input[name="fileInput"]');
const importUserFileList = document.querySelector('#importUserModal .file-list');
const renderFileList = () => {
  importUserFileList.innerHTML = '';
  files.forEach((f, index) => {
    const div = document.createElement('div');
    const flieSpan = document.createElement('span');
    const delBtn = document.createElement('button');

    div.className = 'd-flex align-items-center';
    flieSpan.textContent = `📄 ${f.name}`;
    delBtn.type = 'button';
    delBtn.className = 'ms-4 btn-close';
    delBtn.setAttribute('style', 'font-size: 0.8rem;');
    delBtn.setAttribute('aria-label', 'Close');
    delBtn.addEventListener('click', () => {
      files.splice(index, 1);
      renderFileList();
      $('#importUserSubmit').attr('disabled', true);
    });

    div.appendChild(flieSpan);
    div.appendChild(delBtn);
    importUserFileList.appendChild(div);
  });
};

importUserFileDropZone.addEventListener('click', () => importUserFileInput.click());
importUserFileDropZone.addEventListener('dragover', (e) => {
  e.preventDefault();
  $(this).addClass('bg-light');
});
importUserFileDropZone.addEventListener('dragleave', (e) => {
  e.preventDefault();
  $(this).removeClass('bg-light');
});
importUserFileDropZone.addEventListener('drop', (e) => {
  e.preventDefault();
  $(this).removeClass('bg-light');

  files = Array.from(e.dataTransfer.files).filter((f) => f.name.endsWith('.csv'));
  if (files.length > 1) {
    message.error('不支持多个文件导入');
    return;
  }
  if (files.length === 0) {
    message.error('只允许上传csv文件');
    return;
  }

  $('#importUserSubmit').attr('disabled', false);
  uploadFile.set('file', files[0]);
  renderFileList();
});
importUserFileInput.addEventListener('change', (e) => {
  files = Array.from(e.target.files).filter((f) => f.name.endsWith('.csv'));
  if (files.length === 0) {
    message.error('只允许上传csv文件');
    return;
  }

  $('#importUserSubmit').attr('disabled', false);
  uploadFile.set('file', files[0]);
  renderFileList();
});

$('#importUserSubmit').click(function () {
  uploadFile.set('gid', cgid);
  fetch('/ovpn/user', {
    method: 'POST',
    body: uploadFile,
  })
    .then(async (response) => {
      const body = await response.json();
      if (!response.ok) {
        throw new Error(body?.message || response.text || response.statusText);
      }

      return body;
    })
    .then((data) => {
      $('#importUserModal').modal('hide');
      message.success(data.message);
      vtable.ajax.reload(null, false);
      uploadFile.delete('file');
    })
    .catch((error) => {
      switch (true) {
        case error.message.includes('UNIQUE constraint failed: user.ip_addr'):
          message.error('导入文件有IP已经使用');
          break;
        case error.message.includes('UNIQUE constraint failed: user.username'):
          message.error('导入文件有用户名已存在');
          break;
        default:
          message.error(error.message);
      }
    });
});

// 添加用户
$('#addUserModal form').submit(function () {
  const name = $('#addUserModal input[name="name"]').val();
  const username = $('#addUserModal input[name="username"]').val();
  const password = $('#addUserModal input[name="password"]').val();
  const ipAddr = $('#addUserModal input[name="ipAddr"]').val();
  const expireDate = $('#addUserModal input[name="expireDate"]').val();
  const ovpnConfig = $('#addUserModal select[name="ovpnConfig"]').val() || '';

  request
    .post('/ovpn/user', {
      name,
      username,
      password,
      ipAddr,
      expireDate,
      ovpnConfig,
      gid: cgid,
    })
    .then((data) => {
      vtable.ajax.reload(null, false);
      // vtable.columns.adjust().draw(false);
      $('#addUserModal form').trigger('reset');
      $('#addUserModal').modal('hide');
    });

  return false;
});

$(document).on('keyup', '#addUserModal input[name="ipAddr"]', function () {
  const ipAddr = $('#addUserModal input[name="ipAddr"]').val();
  const regex = /^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$/;

  if (regex.test(ipAddr) || ipAddr.length == 0) {
    $('#addUserModal .form-text').addClass('d-none');
    $('#addUserModal input[name="ipAddr"]').removeClass('border border-danger');
    $('#addUserModal :submit').removeAttr('disabled');
  } else {
    $('#addUserModal .form-text').text('非法IP地址！');
    $('#addUserModal .form-text').addClass('text-danger');
    $('#addUserModal input[name="ipAddr"]').addClass('border border-danger');
    $('#addUserModal .form-text').removeClass('d-none');
    $('#addUserModal :submit').attr('disabled', true);
  }
});

// 编辑用户
$(document).on('click', '#editUser', function () {
  const u = vtable.row($(this).parents('tr')).data();

  $('#editUserModal input[name="id"]').val(u.id);
  $('#editUserModal input[name="name"]').val(u.name);
  $('#editUserModal input[name="username"]').val(u.username);
  $('#editUserModal input[name="ipAddr"]').val(u.ipAddr);
  $('#editUserModal input[name="expireDate"]').val(u.expireDate);

  request.get('/ovpn/client').then((data) => {
    $('#editUserModal select[name="ovpnConfig"]').html(
      data.map((i) => {
        if (i.fullName === u.ovpnConfig) {
          return `<option value="${i.fullName}" selected>${i.name}</option>`;
        }

        return `<option value="${i.fullName}">${i.name}</option>`;
      })
    );
  });

  $('#editUserModal select[name="ovpnConfig"]').val(u.ovpnConfig);

  request.get('/ovpn/group').then((data) => {
    $('#editUserModal select[name="gid"]').html(data.map((i) => `<option value="${i.id}">${i.name}</option>`));
    $('#editUserModal select[name="gid"]').val(cgid);
  });

  const elem = document.querySelector('#editUserModal input[name="expireDate"]');
  const datepicker = new Datepicker(elem, {
    buttonClass: 'btn',
    format: 'yyyy-mm-dd',
    autohide: true,
    language: 'zh-CN',
    orientation: 'top',
    minDate: new Date(),
  });

  datepicker.setDate(new Date(u.expireDate));

  $('#editUserModal').modal('show');
});

$('#editUserModal form').submit(function () {
  const id = $('#editUserModal input[name="id"]').val();
  const name = $('#editUserModal input[name="name"]').val();
  const username = $('#editUserModal input[name="username"]').val();
  const ipAddr = $('#editUserModal input[name="ipAddr"]').val();
  const expireDate = $('#editUserModal input[name="expireDate"]').val();
  const ovpnConfig = $('#editUserModal select[name="ovpnConfig"]').val() || '';
  const gid = $('#editUserModal select[name="gid"]').val();

  request.patch('/ovpn/user', { id, name, username, ipAddr, expireDate, ovpnConfig, gid }).then((data) => {
    vtable.ajax.reload(null, false);
    $('#editUserModal').modal('hide');
    message.success(data.message);
  });

  return false;
});

$(document).on('keyup', '#editUserModal input[name="ipAddr"]', function () {
  const ipAddr = $('#editUserModal input[name="ipAddr"]').val();
  const regex = /^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$/;

  if (regex.test(ipAddr) || ipAddr.length == 0) {
    $('#editUserModal .form-text').addClass('d-none');
    $('#editUserModal input[name="ipAddr"]').removeClass('border border-danger');
    $('#editUserModal :submit').removeAttr('disabled');
  } else {
    $('#editUserModal .form-text').text('非法IP地址！');
    $('#editUserModal .form-text').addClass('text-danger');
    $('#editUserModal input[name="ipAddr"]').addClass('border border-danger');
    $('#editUserModal .form-text').removeClass('d-none');
    $('#editUserModal :submit').attr('disabled', true);
  }
});

// 启用/禁用用户认证
$(document).on('change', '#authUser', function () {
  request
    .post('/ovpn/server', {
      action: 'settings',
      key: 'auth-user',
      value: $(this).is(':checked'),
    })
    .then((data) => {
      message.success(data.message);
    })
    .catch(() => {
      $('#authUser').prop('checked', false);
    });
});

// 复制密码
$(document).on('click', '#copyPass', function () {
  copyToClipboard(this.previousSibling.previousSibling.value?.trim());

  const icon = $(this).html();
  $(this).html(`
    <svg width="1em" height="1em" fill="currentColor" viewBox="0 0 16 16">
      <path d="M13.854 3.646a.5.5 0 0 1 0 .708l-7 7a.5.5 0 0 1-.708 0l-3.5-3.5a.5.5 0 1 1 .708-.708L6.5 10.293l6.646-6.647a.5.5 0 0 1 .708 0z"/>
    </svg>`);
  $(this).addClass('text-success');
  $(this).attr('disabled', true);

  setTimeout(() => {
    $(this).html(icon);
    $(this).removeClass('text-success');
    $(this).attr('disabled', false);
  }, 1500);
});

// 禁用用户
$(document).on('click', '#disableUser', function () {
  const id = vtable.row($(this).parents('tr')).data().id;

  request.patch('/ovpn/user', { id, isEnable: false }).then((data) => {
    message.success(data.message);
    vtable.ajax.reload(null, false);
  });
});

// 启用用户
$(document).on('click', '#enableUser', function () {
  const id = vtable.row($(this).parents('tr')).data().id;

  request.patch('/ovpn/user', { id, isEnable: true }).then((data) => {
    message.success(data.message);
    vtable.ajax.reload(null, false);
  });
});

// 重置MFA
$(document).on('click', '#resetMfa', function () {
  const id = vtable.row($(this).parents('tr')).data().id;
  $('#resetMfaInfoModal input[name="id"]').val(id);
  $('#resetMfaInfoModal').modal('show');
});

$('#resetMfaInfoSumbit').click(function () {
  const id = $('#resetMfaInfoModal input[name="id"]').val();
  request.delete(`/client/mfa/${id}`).then((data) => {
    $('#resetMfaInfoModal').modal('hide');
    message.success('MFA已重置');
    vtable.ajax.reload(null, false);
  });
});

// 重置密码
$(document).on('click', '#resetPass', function () {
  const id = vtable.row($(this).parents('tr')).data().id;
  const username = vtable.row($(this).parents('tr')).data().username;
  $('#resetPassModal input[name="id"]').val(id);
  $('#resetPassModal input[name="username"]').val(username);

  $('#resetPassModal').modal('show');
});

$(document).on('keyup', '#resetPassModal input[name="newPassAgain"]', function () {
  const newPss = $('#resetPassModal input[name="newPass"]').val();
  const newPassAgain = $('#resetPassModal input[name="newPassAgain"]').val();

  if (newPassAgain == newPss) {
    $('#resetPassModal .form-text').addClass('d-none');
    $('#resetPassModal input[name="newPassAgain"]').removeClass('border border-danger');
    $('#resetPassSumbit').removeAttr('disabled');
  } else {
    $('#resetPassModal .form-text').text('密码不一致！');
    $('#resetPassModal .form-text').addClass('text-danger');
    $('#resetPassModal input[name="newPassAgain"]').addClass('border border-danger');
    $('#resetPassModal .form-text').removeClass('d-none');
    $('#resetPassSumbit').attr('disabled', true);
  }
});

$('#resetPassModal form').submit(function () {
  const id = $('#resetPassModal input[name="id"]').val();
  const newPass = $('#resetPassModal input[name="newPassAgain"]').val();

  request.patch('/ovpn/user', { id, password: newPass }).then(() => {
    vtable.ajax.reload(null, false);
    $('#resetPassModal form').trigger('reset');
    $('#resetPassModal').modal('hide');
    message.success('密码重置成功');
  });

  return false;
});

// 树形菜单
let currentNode;
let expandedIds = new Set();
const max_depth = 3;
const treeMenu = document.getElementById('treeMenu');

function buildTree(data, parentId = null, depth = 0) {
  return data
    .filter((item) => item.parent_id === parentId)
    .map((item) => ({
      ...item,
      depth: depth,
      children: buildTree(data, item.id, depth + 1),
    }));
}

function renderTree(nodes, container) {
  nodes.forEach((node) => {
    const li = document.createElement('li');

    const hasChildren = node.children && node.children.length > 0;
    const isExpanded = expandedIds.has(node.id);
    const toggleIcon = hasChildren ? 'fa-chevron-right' : '';

    const itemDiv = document.createElement('div');
    itemDiv.className = 'tree-item';
    itemDiv.dataset.id = node.id;

    itemDiv.innerHTML = `
          <span class="tree-toggle ${hasChildren ? '' : 'hidden'} ${isExpanded ? 'expanded' : ''}">
              <i class="fas ${toggleIcon}"></i>
          </span>
          <i class="fas ${hasChildren ? (isExpanded ? 'fa-folder-open' : 'fa-folder') : 'fa-folder'} text-warning"></i>
          <span class="text-truncate">${node.name}</span>
      `;

    li.appendChild(itemDiv);

    if (hasChildren) {
      const childUl = document.createElement('ul');
      childUl.className = 'tree-menu';
      childUl.style.display = isExpanded ? 'block' : 'none';
      renderTree(node.children, childUl);
      li.appendChild(childUl);
    }

    itemDiv.addEventListener('contextmenu', (e) => {
      e.preventDefault();
      handleContextMenu(e, node);
    });

    itemDiv.addEventListener('click', (e) => {
      e.stopPropagation();
      contextMenu.style.display = 'none';

      document.querySelectorAll('.tree-item').forEach((item) => item.classList.remove('selected'));
      itemDiv.classList.add('selected');

      cgid = node.id;
      vtable.ajax.reload(null, false);
    });

    itemDiv.querySelector('.tree-toggle').addEventListener('click', (e) => {
      e.stopPropagation();

      if (hasChildren) {
        const toggle = itemDiv.querySelector('.tree-toggle');
        const childUl = li.querySelector('.tree-menu');
        const folderIcon = itemDiv.querySelector('.fa-folder, .fa-folder-open');
        const isExpanded = toggle.classList.contains('expanded');

        if (isExpanded) {
          toggle.classList.remove('expanded');
          childUl.style.display = 'none';
          expandedIds.delete(node.id);

          folderIcon.classList.remove('fa-folder-open');
          folderIcon.classList.add('fa-folder');
        } else {
          toggle.classList.add('expanded');
          childUl.style.display = 'block';
          expandedIds.add(node.id);

          folderIcon.classList.remove('fa-folder');
          folderIcon.classList.add('fa-folder-open');
        }
      }
    });

    container.appendChild(li);
  });
}

function refreshTree(data) {
  treeMenu.innerHTML = '';
  const tree = buildTree(data);
  if (expandedIds.size === 0 && tree.length > 0) {
    expandedIds.add(tree[0].id);
  }
  renderTree(tree, treeMenu);
}

const contextMenu = document.getElementById('contextMenu');
const menuAdd = document.getElementById('menuAdd');
const menuEdit = document.getElementById('menuEdit');
const menuExport = document.getElementById('menuExport');
const menuDelete = document.getElementById('menuDelete');
const menuVpnConfig = document.getElementById('menuVpnConfig');

function handleContextMenu(e, node) {
  currentNode = node;

  if (node.depth < max_depth) {
    menuAdd.style.opacity = '1';
    menuAdd.style.pointerEvents = 'auto';
  } else {
    menuAdd.style.opacity = '0.5';
    menuAdd.style.pointerEvents = 'none';
  }

  if (node.parent_id === null) {
    menuDelete.style.opacity = '0.5';
    menuDelete.style.pointerEvents = 'none';
    menuVpnConfig.style.opacity = '0.5';
    menuVpnConfig.style.pointerEvents = 'none';
  } else {
    menuDelete.style.opacity = '1';
    menuDelete.style.pointerEvents = 'auto';
    menuVpnConfig.style.opacity = '1';
    menuVpnConfig.style.pointerEvents = 'auto';
  }

  contextMenu.style.display = 'block';
  contextMenu.style.left = `${e.pageX}px`;
  contextMenu.style.top = `${e.pageY}px`;
}

menuAdd.addEventListener('click', () => {
  contextMenu.style.display = 'none';
  setTimeout(() => {
    const name = prompt('请输入新节点名称：');
    if (name) {
      request.post('/ovpn/group', { name, parent_id: currentNode.id }).then(() => {
        expandedIds.add(currentNode.id);
        getTreeData();
      });
    }
  }, 0);
});

menuEdit.addEventListener('click', () => {
  contextMenu.style.display = 'none';
  setTimeout(() => {
    const newName = prompt('修改节点名称为：', currentNode.name);
    if (newName) {
      request.patch('/ovpn/group', { id: currentNode.id, name: newName }).then(() => {
        getTreeData();
      });
    }
  }, 0);
});

menuExport.addEventListener('click', () => {
  contextMenu.style.display = 'none';
  window.location.href = `/ovpn/user/export?gid=${cgid}`;
});

menuVpnConfig.addEventListener('click', () => {
  contextMenu.style.display = 'none';
  request.get(`/ovpn/group/${currentNode.id}`).then((data) => {
    $('#treeVpnConfigModal textarea[name="config"]').val(data.config?.replace(/\\n/g, '\n'));
    $('#treeVpnConfigModal').modal('show');
  });
});

menuDelete.addEventListener('click', () => {
  contextMenu.style.display = 'none';
  setTimeout(() => {
    if (confirm(`确定要删除 "${currentNode.name}" 及其节点下所有子节点和账号数据吗？`)) {
      request.delete(`/ovpn/group/${currentNode.id}`).then(() => {
        getTreeData();
        vtable.ajax.reload(null, false);
      });
    }
  }, 0);
});

document.addEventListener('click', (e) => {
  if (e.target.offsetParent !== contextMenu) {
    contextMenu.style.display = 'none';
  }
});

$('#treeVpnConfigSumbit').click(function () {
  const config = $('#treeVpnConfigModal textarea[name="config"]').val();
  request.patch('/ovpn/group', { id: currentNode.id, config: config?.trim()?.replace(/\n/g, '\\n') }).then(() => {
    $('#treeVpnConfigModal textarea[name="config"]').val('');
    $('#treeVpnConfigModal').modal('hide');
    message.success('设置VPN配置成功');
  });
});

export function getTreeData() {
  request.get('/ovpn/group').then((data) => {
    refreshTree(data);
  });
}
