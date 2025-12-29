tables.history = {
  columns: [
    { title: '用户名', data: 'username' },
    { title: '客户端', data: 'common_name' },
    { title: 'VPN IP', data: 'vip' },
    { title: '用户 IP', data: 'rip' },
    { title: '下载流量', data: 'bytes_received' },
    { title: '上传流量', data: 'bytes_sent' },
    { title: '上线时间', data: 'time_unix' },
    { title: '在线时长', data: 'time_duration' },
  ],
  order: [[6, 'desc']],
  processing: true,
  serverSide: true,
  search: {
    return: true,
  },
  dom:
    "<'row align-items-center'<'col d-flex'f><'col d-flex justify-content-center toolbar'><'col d-flex justify-content-end'B>>" +
    "<'row'<'col-sm-12'tr>>" +
    "<'d-flex justify-content-between align-items-center'lip>",
  fnInitComplete: function (oSettings, data) {
    $('#vtable_wrapper div.toolbar').html(
      `<div id="datepicker">
          <div class="input-group input-group-sm">
            <input type="text" class="form-control text-center" name="start" />
            <span class="input-group-text">to</span>
            <input type="text" class="form-control text-center" name="end" />
          </div>
        </div>
        `
    );

    const elem = document.getElementById('datepicker');
    const rangepicker = new DateRangePicker(elem, {
      buttonClass: 'btn',
      container: elem,
      format: 'yyyy-mm-dd',
      autohide: true,
      language: 'zh-CN',
    });

    rangepicker.setDates(lastMonth, now);

    elem.addEventListener('changeDate', (e) => {
      qt = rangepicker.getDates().map((d, i) => {
        if (d instanceof Date) {
          if (i == 1) {
            d.setHours(23, 59, 59, 0);
          }

          return Date.parse(d) / 1000;
        }

        return qt[i];
      });

      vtable.ajax.reload();
    });
  },
  buttons: {
    dom: {
      button: { className: 'btn btn-sm' },
    },
    buttons: [
      {
        text: '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" viewBox="0 0 16 16"> <path fill-rule="evenodd" d="M8 3a5 5 0 1 0 4.546 2.914.5.5 0 0 1 .908-.417A6 6 0 1 1 8 2z"/> <path d="M8 4.466V.534a.25.25 0 0 1 .41-.192l2.36 1.966c.12.1.12.284 0 .384L8.41 4.658A.25.25 0 0 1 8 4.466"/> </svg> 刷新 ',
        className: 'btn-primary',
        action: () => vtable.ajax.reload(),
      },
    ],
  },
  ajax: function (data, callback, settings) {
    const orderColumn = data.columns[data.order[0].column].data;
    const order = data.order[0].dir;
    const params = {
      draw: data.draw,
      offset: data.start,
      limit: data.length,
      orderColumn: orderColumn,
      order: order,
      search: data.search.value,
      qt: qt.join(),
    };

    request.get(`/ovpn/history?${new URLSearchParams(params).toString()}`).then((data) => callback(data));
  },
};
