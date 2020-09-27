import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import api from '../core/api-service.js';

export default class FileHexView extends React.Component {
    static propTypes = {
        selectedRow: PropTypes.object,
        client: PropTypes.object,
    };

    state = {
        page: 1,
        pageCount: 1,
        rows: 25,
        columns: 0x14,
        hexDataRows: [],
        chunkSize: 1024,

        // Offset into the file.
        offset: 0,
    }

    componentDidMount = () => {
        this.fetchText_();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let selectedRow = this.props.selectedRow && this.props.selectedRow._id;
        let old_row = prevProps.selectedRow && prevProps.selectedRow._id;

        if (selectedRow != old_row) {
            this.fetchText_();
        };
    }

    fetchText_ = () => {
        let client_id = this.props.client && this.props.client.client_id;
        if (!client_id) {
            return;
        }

        var download = this.props.selectedRow.Download;
        if (!download) {
            return;
        }

        var filePath = download.vfs_path;
        var total_size = this.props.selectedRow.Size || 0;
        var chunkSize = this.state.rows * this.state.columns;
        let pageCount = Math.ceil(total_size / chunkSize);
        var url = 'api/v1/DownloadVFSFile';
        var params = {
            offset: this.state.offset,
            length: chunkSize,
            vfs_path: filePath,
            client_id: client_id,
        };

        api.get(url, params).then(function(response) {
            this.parseFileContentToHexRepresentation_(response.data);
        }.bind(this), function() {
            this.setState({hexDataRows: []});
        }.bind(this));
    };

    parseFileContentToHexRepresentation_ = (fileContent) => {
        if (!fileContent) {
            return;
        }
        let hexDataRows = [];

        for(var i = 0; i < this.state.rows; i++){
            var rowOffset = this.offset_ + (i * this.state.columns);
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

        this.setState({hexDataRows: hexDataRows});
    };


    render() {
        if (this.state.hexDataRows) {
            return <div className="panel hexdump">
                     Loading...
                   </div>;
        }

        return (
            <div>
              <div>
                <ul uib-pagination
                    total-items="controller.pageCount" items-per-page="1"
                    rotate="false" boundary-links="true" max_size="10"
                    ng-model="controller.page">
                </ul>

                <div className="panel hexdump">
                  <div className="monospace">
                    <table>
                      <tr>
                        <th>Offset</th>
                        <th>00 01 02 03 04 05 06 07 08 09 0a 0b 0c 0d 0e 0f 10 11 12 13</th>
                        <th></th>
                      </tr>
                      <tr>
                        <td>
                          <table className="offset-area">
                            { _.map(this.state.hexDataRows, function(row, idx) {
                                return <tr ng-repeat="row in controller.hexDataRows">
                                  <td className="offset">
                                    { row.offset }
                                  </td>
                                </tr>; })}
                          </table>
                        </td>
                        <td>
                          <table className="hex-area">
                            { _.map(this.state.hexDataRows, function(row, idx) {
                                return <tr ng-repeat="row in controller.hexDataRows">
                                  <td>
                                    { _.map(row.data_row, function(x, idx) {
                                        return <span ng-repeat="x in row.data_row track by $index">
                                        { x }
                                        </span>;
                                    })}
                                  </td>
                                </tr>; })}
                          </table>
                        </td>
                        <td>
                          <table className="content-area">
                            { _.map(this.state.hexDataRows, function(row, idx) {
                                return <tr ng-repeat="row in controller.hexDataRows">
                                  <td className="data">
                                    { row.safe_data }
                                  </td>
                                </tr>;
                            })}
                          </table>
                        </td>
                      </tr>
                    </table>
                  </div>
                </div>
              </div>
            </div>
        );
    }
};
