import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';
import api from '../core/api-service.js';
import Pagination from '../bootstrap/pagination/index.js';
import Spinner from '../utils/spinner.js';
import utils from './utils.js';
import VeloAce from '../core/ace.js';

import "./file-hex-view.css";


const pagesize = 100 * 1024;

export default class FileTextView extends React.Component {
    static propTypes = {
        node: PropTypes.object,
        client: PropTypes.object,
    };

    state = {
        page: 0,
        rawdata: "",
        loading: false,
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
        var url = 'v1/DownloadVFSFile';
        var params = {
            offset: page * pagesize,
            length: pagesize,
            vfs_path: filePath,
            client_id: client_id,
        };

        this.setState({loading: true});
        api.get_blob(url, params).then(function(response) {
            this.parseFileContentToTextRepresentation_(response || "", page);
        }.bind(this), function() {
            this.setState({hexDataRows: [], loading: false, page: page});
        }.bind(this));
    };

    parseFileContentToTextRepresentation_ = (fileContent, page) => {
        let rawdata = fileContent.replace(/[^\x20-\x7f\r\n]/g, '.');
        this.setState({rawdata: rawdata, loading: false, page: page});
    };


    render() {
        let selectedRow = utils.getSelectedRow(this.props.node);
        let mtime = selectedRow && selectedRow.Download && selectedRow.Download.mtime;
        if (!mtime) {
            return <h5 className="no-content">File has no data, please collect file first.</h5>;
        }
        var total_size = selectedRow.Size || 0;
        let pageCount = Math.ceil(total_size / pagesize);
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
                e.preventDefault();
                e.stopPropagation();
            },
        };

        return (
            <div>
              <Spinner loading={this.state.loading}/>
              <div className="file-hex-view">
                <Pagination {...paginationConfig} />
                <VeloAce
                  text={this.state.rawdata}
                  mode="text"
                  options={{readOnly: true}}
                />
              </div>
            </div>
        );
    }
};
