import React from 'react';
import PropTypes from 'prop-types';

import api from '../core/api-service.js';
import Pagination from '../bootstrap/pagination/index.js';

import VeloAce from '../core/ace.js';

import "./file-hex-view.css";


const pagesize = 100 * 1024;

export default class FileTextView extends React.Component {
    static propTypes = {
        selectedRow: PropTypes.object,
        client: PropTypes.object,
    };

    state = {
        page: 0,
        rawdata: "",
        loading: true,
    }

    componentDidMount = () => {
        this.fetchText_(0);
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let selectedRow = this.props.selectedRow && this.props.selectedRow._id;
        let old_row = prevProps.selectedRow && prevProps.selectedRow._id;

        if (selectedRow !== old_row) {
            this.fetchText_(this.state.page);
        };
    }

    fetchText_ = (page) => {
        let client_id = this.props.client && this.props.client.client_id;
        if (!client_id) {
            return;
        }

        var download = this.props.selectedRow.Download;
        if (!download) {
            return;
        }

        var filePath = download.vfs_path;
        if (!filePath) {
            return;
        }

        var url = 'api/v1/DownloadVFSFile';

        var params = {
            offset: page * pagesize,
            length: pagesize,
            vfs_path: filePath,
            client_id: client_id,
        };

        this.setState({loading: true});

        api.get(url, params).then(function(response) {
            this.parseFileContentToTextRepresentation_(response.data || "", page);
        }.bind(this), function() {
            this.setState({hexDataRows: [], loading: false});
        }.bind(this));
    };

    parseFileContentToTextRepresentation_ = (fileContent, page) => {
        let rawdata = fileContent.replace(/[^\x20-\x7f\r\n]/g, '.');
        this.setState({rawdata: rawdata, loading: false});
    };


    render() {
        if (this.state.loading) {
            return <div className="panel hexdump">
                     Loading...
                   </div>;
        }

        var total_size = this.props.selectedRow.Size || 0;
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
                this.fetchText_(page - 1);
                e.preventDefault();
                e.stopPropagation();
            },
        };

        return (
            <div>
              <div className="file-hex-view">
                { pageCount && <Pagination {...paginationConfig}/> }
                <VeloAce
                  text={this.state.rawdata}
                  mode="text"
                />
              </div>
            </div>
        );
    }
};
