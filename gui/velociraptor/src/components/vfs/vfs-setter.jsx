import React, { Component } from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import { withRouter }  from "react-router-dom";

import { SplitPathComponents, DecodePathInURL } from '../utils/paths.jsx';

// A component that syncs a client id to a client record.
class VFSSetterFromRoute extends Component {
    static propTypes = {
        vfs_path: PropTypes.array,
        updateVFSPath: PropTypes.func.isRequired,

        // React router props.
        match: PropTypes.object,
    }

    componentDidUpdate() {
        this.maybeUpdateVFSPath();
    }

    componentDidMount() {
        this.maybeUpdateVFSPath();
    }

    maybeUpdateVFSPath() {
        if (!this.props.match || !this.props.match.params) {
            return;
        }

        let url_vfs = SplitPathComponents(DecodePathInURL(this.props.match.params.vfs_path || ""));
        let props_vfs = this.props.vfs_path;

        if (!_.isEqual(url_vfs, props_vfs)) {
            this.props.updateVFSPath(url_vfs);
        }
    }

    render() {
        return <></>;
    }
}

export default withRouter(VFSSetterFromRoute);
