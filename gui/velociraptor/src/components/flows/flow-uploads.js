import React from 'react';
import PropTypes from 'prop-types';

import VeloTable, { PrepareData } from '../core/table.js';

import api from '../core/api-service.js';

const MAX_ROWS_PER_TABLE = 500;

export default class FlowUploads extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
    };

    componentDidUpdate(prevProps, prevState, snapshot) {
        let prev_flow_id = prevProps.flow && prevProps.flow.session_id;
        if (this.props.flow.session_id != prev_flow_id) {
            this.fetchRows();
        }
    }

    state = {
        loading: true,
        pageData: {},
    }

    fetchRows = () => {
        let params = {
            client_id: this.props.flow.client_id,
            flow_id: this.props.flow.session_id,
            type: "uploads",
            start_row: 0,
            rows: MAX_ROWS_PER_TABLE,
        };

        this.setState({loading: true});
        api.get("api/v1/GetTable", params).then((response) => {
            this.setState({loading: false, pageData: PrepareData(response.data)});
        }).catch(() => {
            this.setState({loading: false, pageData: {}});
        });
    }

    downloadFile = (e) => {
        e.stopPropagation();
        e.preventDefault();
    }

    render() {
        if (!this.state.pageData || !this.state.pageData.columns) {
            return (
                <div className="card panel" >
                  <h5 className="card-header">Files uploaded</h5>
                  <div className="card-body">
                    <div>No data available</div>
                  </div>
                </div>
            );
        }

        let renderers = {
            // Let users directly download the file without having to
            // make a zip file.
            vfs_path: (cell, row, rowIndex) => {
                return (
                    <a href={"api/v1/DownloadVFSFile?client_id=" +
                             this.props.flow.client_id +
                             "&vfs_path=" + encodeURIComponent(cell) }>
                      {cell}
                    </a>
                );
            },
        };

        return (
            <div className="card panel" >
              <h5 className="card-header">Files uploaded</h5>
              <div className="card-body">
                <VeloTable
                  className="col-12"
                  renderers={renderers}
                  rows={this.state.pageData.rows}
                  columns={this.state.pageData.columns} />
              </div>
            </div>
        );
    }
};
