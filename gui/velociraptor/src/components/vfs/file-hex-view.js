import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import api from '../core/api-service.js';
import Pagination from '../bootstrap/pagination/index.js';
import Spinner from '../utils/spinner.js';
import utils from './utils.js';
import axios from 'axios';
import "./file-hex-view.css";
import T from '../i8n/i8n.js';

export default class FileHexView extends React.Component {
    static propTypes = {
        node: PropTypes.object,
        client: PropTypes.object,
    };

    state = {
        page: 0,
        rows: 25,
        columns: 0x10,
        hexDataRows: [],
        loading: true,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.fetchText_(0);
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        // Update the view when
        // 1. Selected node changes (file list was selected).
        // 2. VFS path changes (tree navigated away).
        // 3. node version changes (file was refreshed).
        if (prevProps.node.selected !== this.props.node.selected ||
            !_.isEqual(prevProps.node.path, this.props.node.path) ||
            prevProps.node.version !== this.props.node.version) {
            this.fetchText_(this.state.page);
        };
    }

    fetchText_ = (page) => {
        let selectedRow = utils.getSelectedRow(this.props.node);
        let client_id = this.props.client && this.props.client.client_id;
        if (!client_id) {
            return;
        }

        let name = selectedRow && selectedRow.Name;
        if (!name) {
            return;
        }

        let fileComponents = this.props.node.path.slice();
        if (_.isEmpty(fileComponents)) {
            return;
        }
        fileComponents.push(name);

        let vfs_path = selectedRow.Download && selectedRow.Download.vfs_path;
        var chunkSize = this.state.rows * this.state.columns;
        var url = 'v1/DownloadVFSFile';

        // vfsFileDownloadRequest struct schema in /api/download.go
        var params = {
            offset: page * chunkSize,
            length: chunkSize,
            components: fileComponents,
            client_id: client_id,
            vfs_path: vfs_path,
        };

        this.setState({loading: true});
        api.get_blob(url, params, this.source.token).then(buffer=> {
            const view = new Uint8Array(buffer);
            this.parseFileContentToHexRepresentation_(view, page);
        });
    };

    parseFileContentToHexRepresentation_ = (intArray, page) => {
        if (!intArray) {
            intArray = "";
        }
        let hexDataRows = [];
        var chunkSize = this.state.rows * this.state.columns;

        for(var i = 0; i < this.state.rows; i++){
            let offset = page * chunkSize;
            var rowOffset = offset + (i * this.state.columns);
            var data = intArray.slice(i * this.state.columns, (i+1)*this.state.columns);
            var data_row = [];
            var safe_data = "";
            for (var j = 0; j < data.length; j++) {
                var char = data[j].toString(16);
                if (data[j] > 0x20 && data[j] < 0x7f) {
                    safe_data += String.fromCharCode(data[j]);
                } else {
                    safe_data += ".";
                };
                data_row.push(('0' + char).substr(-2)); // add leading zero if necessary
            };

            hexDataRows.push({
                offset: rowOffset,
                data_row: data_row,
                data: data,
                safe_data: safe_data,
            });
        }

        this.setState({hexDataRows: hexDataRows, loading: false});
    };


    render() {
        let selectedRow = utils.getSelectedRow(this.props.node);
        let mtime = selectedRow && selectedRow.Download && selectedRow.Download.mtime;
        if (!mtime) {
            return <h5 className="no-content">{T("File has no data, please collect file first.")}</h5>;
        }

        var total_size = selectedRow.Size || 0;
        var chunkSize = this.state.rows * this.state.columns;
        let pageCount = Math.ceil(total_size / chunkSize);
        let paginationConfig = {
            totalPages: pageCount,
            currentPage: this.state.page + 1,
            showMax: 5,
            size: "sm",
            threeDots: true,
            center: true,
            prevNext: true,
            shadow: true,
            onClick: (page, e) => {
                this.setState({page: page - 1});
                this.fetchText_(page - 1);
                e.preventDefault();
                e.stopPropagation();
            },
        };

        let hexArea = this.state.loading ? T("Loading") :
            <table className="hex-area">
              <tbody>
                { _.map(this.state.hexDataRows, function(row, idx) {
                    return <tr key={idx}>
                             <td>
                               { _.map(row.data_row, function(x, idx) {
                                   return <span key={idx}>{ x }&nbsp;</span>;
                               })}
                             </td>
                           </tr>; })
                }
              </tbody>
            </table>;

        let contextArea = this.state.loading ? "" :
            <table className="content-area">
              <tbody>
                { _.map(this.state.hexDataRows, function(row, idx) {
                    return <tr key={idx}><td className="data">{ row.safe_data }</td></tr>;
                })}
              </tbody>
            </table>;

        return (
            <div>
              <Spinner loading={this.state.loading}/>
              <div className="file-hex-view">
                { pageCount && <Pagination {...paginationConfig}/> }

                <div className="panel hexdump">
                  <div className="monospace">
                    <table>
                      <thead>
                        <tr>
                          <th className="hex_column offset">{T("Offset")}</th>
                          <th className="hex_column">00 01 02 03 04 05 06 07 08 09 0a 0b 0c 0d 0e 0f </th>
                          <th></th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr>
                          <td className="hex_column offset">
                            <table className="offset-area">
                              <tbody>
                                { _.map(this.state.hexDataRows, function(row, idx) {
                                    return <tr key={idx}>
                                             <td className="offset">
                                               0x{ row.offset.toString(16) }
                                             </td>
                                           </tr>; })}
                              </tbody>
                            </table>
                          </td>
                          <td className="hex_column">
                            { hexArea }
                          </td>
                          <td className="hex_column">
                            { contextArea }
                          </td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>
            </div>
        );
    }
};
