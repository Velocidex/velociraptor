import _ from 'lodash';

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import URLViewer from "../utils/url.jsx";
import Alert from 'react-bootstrap/Alert';
import api from '../core/api-service.jsx';
import T from '../i8n/i8n.jsx';


export default class Download extends Component {
    static propTypes = {
        fs_components: PropTypes.array,
        filename: PropTypes.string,
        text: PropTypes.any,
    }

    render() {
        if (_.isEmpty(this.props.fs_components)) {
            return <Alert variant="warning">{this.props.text}</Alert>;
        };

        let url = api.href(
            "/api/v1/DownloadVFSFile",
            {"fs_components": this.props.fs_components,
             vfs_path: this.props.filename || T("Download")});

        return (
            <URLViewer url={url} desc={this.props.text} />
        );
    }
}
