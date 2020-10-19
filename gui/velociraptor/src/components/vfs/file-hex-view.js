import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import api from '../core/api-service.js';
import Pagination from '../bootstrap/pagination/index.js';
import Spinner from '../utils/spinner.js';
import utils from './utils.js';

import "./file-hex-view.css";

export default class FileHexView extends React.Component {
    static propTypes = {
        node: PropTypes.object,
        client: PropTypes.object,
    };

    state = {
        page: 0,
        rows: 25,
        columns: 0x14,
        hexDataRows: [],
        loading: true,
    }

    componentDidMount = () => {
        this.fetchText_(0);
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

        var download = selectedRow && selectedRow.Download;
        if (!download) {
            return;
        }

        var filePath = download.vfs_path;
        if (!filePath) {
            return;
        }

        var chunkSize = this.state.rows * this.state.columns;
        var url = 'v1/DownloadVFSFile';

        var params = {
            offset: page * chunkSize,
            length: chunkSize,
            vfs_path: filePath,
            client_id: client_id,
        };

        this.setState({loading: true});
        api.get_blob(url, params).then((response) => {
            this.parseFileContentToHexRepresentation_(response, page);
        });
    };

    parseFileContentToHexRepresentation_ = (fileContent, page) => {
        if (!fileContent) {
            fileContent = "";
        }
        let hexDataRows = [];
        var chunkSize = this.state.rows * this.state.columns;

        for(var i = 0; i < this.state.rows; i++){
            let offset = page * chunkSize;
            var rowOffset = offset + (i * this.state.columns);
            var data = fileContent.substr(i * this.state.columns, this.state.columns);
            var data_row = [];
            for (var j = 0; j < data.length; j++) {
                var char = data.charCodeAt(j).toString(16);
                data_row.push(('0' + char).substr(-2)); // add leading zero if necessary
            };

            hexDataRows.push({
                offset: rowOffset,
                data_row: data_row,
                data: data,
                safe_data: data.replace(/[^\x20-\x7f]/g, '.'),
            });
        }

        this.setState({hexDataRows: hexDataRows, loading: false});
    };


    render() {
        let selectedRow = utils.getSelectedRow(this.props.node);
        let mtime = selectedRow && selectedRow.Download && selectedRow.Download.mtime;
        if (!mtime) {
            return <h5 className="no-content">File has no data, please collect file first.</h5>;
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

        let hexArea = this.state.loading ? "Loading" :
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
                          <th>Offset</th>
                          <th>00 01 02 03 04 05 06 07 08 09 0a 0b 0c 0d 0e 0f 10 11 12 13</th>
                          <th></th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr>
                          <td>
                            <table className="offset-area">
                              <tbody>
                                { _.map(this.state.hexDataRows, function(row, idx) {
                                    return <tr key={idx}>
                                             <td className="offset">
                                               { row.offset }
                                             </td>
                                           </tr>; })}
                              </tbody>
                            </table>
                          </td>
                          <td>
                            { hexArea }
                          </td>
                          <td>
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
